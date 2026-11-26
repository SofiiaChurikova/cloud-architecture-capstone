package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	p99Query     = `histogram_quantile(0.99, sum by (le) (rate(kv_request_duration_seconds_bucket{job="coordinator", op=~"write|delete"}[1m])))`
	trafficQuery = `sum(rate(kv_requests_total{job="coordinator", op=~"write|delete"}[1m]))`
	errorsQuery  = `sum(rate(kv_requests_total{job="coordinator", op=~"write|delete", status=~"5.."}[1m]))`
	upQuery      = `up{job="coordinator"}`
	maxErrors    = 0.01
)

type config struct {
	up, down   float64
	upFor      time.Duration
	downFor    time.Duration
	cooldown   time.Duration
	minTraffic float64
	downRate   float64
}

type observation struct {
	p99       float64
	traffic   float64
	errors    float64
	noData    bool
	shards    int
	moving    bool
	stackBusy bool
}

type decider struct {
	cfg        config
	aboveSince time.Time
	belowSince time.Time
	lastAction time.Time
}

func (d *decider) step(now time.Time, o observation) (string, string) {
	if o.moving || o.stackBusy {
		d.aboveSince, d.belowSince = time.Time{}, time.Time{}
		return "", "a shard move or a stack update is running"
	}
	if o.noData || (o.traffic >= d.cfg.minTraffic && math.IsNaN(o.p99)) {
		d.aboveSince, d.belowSince = time.Time{}, time.Time{}
		return "", "no data from Prometheus, doing nothing"
	}
	idle := o.traffic < d.cfg.minTraffic
	high := !idle && (o.p99 > d.cfg.up || o.errors/o.traffic > maxErrors)
	low := idle || (o.p99 < d.cfg.down && o.traffic < d.cfg.downRate)
	d.aboveSince = since(d.aboveSince, high, now)
	d.belowSince = since(d.belowSince, low, now)

	if !d.lastAction.IsZero() && now.Sub(d.lastAction) < d.cfg.cooldown {
		return "", "cooldown"
	}
	switch {
	case o.shards == 1 && high && now.Sub(d.aboveSince) >= d.cfg.upFor:
		d.act(now)
		return "up", fmt.Sprintf("write p99 above %.0f ms or more than 1%% errors for %s", d.cfg.up*1000, d.cfg.upFor)
	case o.shards >= 2 && low && now.Sub(d.belowSince) >= d.cfg.downFor:
		d.act(now)
		return "down", fmt.Sprintf("write p99 below %.0f ms and under %.0f writes/s (or no traffic) for %s", d.cfg.down*1000, d.cfg.downRate, d.cfg.downFor)
	}
	return "", ""
}

func (d *decider) act(now time.Time) {
	d.lastAction = now
	d.aboveSince, d.belowSince = time.Time{}, time.Time{}
}

func since(t time.Time, cond bool, now time.Time) time.Time {
	if !cond {
		return time.Time{}
	}
	if t.IsZero() {
		return now
	}
	return t
}

type cloud interface {
	stackBusy() (bool, error)
	setShardCount(n int) error
}

type scaler struct {
	d        *decider
	prom     string
	kv       string
	token    string
	drainID  string
	http     *http.Client
	cloud    cloud
	dryRun   bool
	pollWait time.Duration
	log      *slog.Logger
}

func (s *scaler) tick(ctx context.Context, now time.Time) error {
	var o observation
	var err error
	if o.p99, err = s.promValue(ctx, p99Query); err != nil {
		return err
	}
	if o.traffic, err = s.promValue(ctx, trafficQuery); err != nil {
		return err
	}
	if math.IsNaN(o.traffic) {
		o.traffic = 0
	}
	if o.errors, err = s.promValue(ctx, errorsQuery); err != nil {
		return err
	}
	if math.IsNaN(o.errors) {
		o.errors = 0
	}
	up, err := s.promValue(ctx, upQuery)
	if err != nil {
		return err
	}
	o.noData = math.IsNaN(up) || up < 1
	var health struct {
		Shards    int    `json:"shards"`
		Migration string `json:"migration"`
	}
	if err := s.getJSON(ctx, s.kv+"/health", &health); err != nil {
		return err
	}
	o.shards, o.moving = health.Shards, health.Migration != "idle"
	if !s.dryRun {
		if o.stackBusy, err = s.cloud.stackBusy(); err != nil {
			return err
		}
	}

	action, reason := s.d.step(now, o)
	s.log.Info("tick", "p99_ms", round(o.p99*1000), "writes_per_s", round(o.traffic), "shards", o.shards,
		"errors_per_s", round(o.errors), "no_data", o.noData, "moving", o.moving, "stack_busy", o.stackBusy, "action", action, "reason", reason, "dry_run", s.dryRun)
	if action == "" || s.dryRun {
		return nil
	}
	defer func() { s.d.lastAction = time.Now() }()
	switch action {
	case "up":
		return s.cloud.setShardCount(2)
	case "down":
		if err := s.post(ctx, s.kv+"/admin/shards/"+s.drainID+"/drain"); err != nil {
			return err
		}
		wctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
		defer cancel()
		if err := s.waitIdle(wctx); err != nil {
			return err
		}
		return s.cloud.setShardCount(1)
	}
	return nil
}

func (s *scaler) waitIdle(ctx context.Context) error {
	for {
		var st struct {
			State  string `json:"state"`
			Copied int64  `json:"copied"`
			Total  int64  `json:"total"`
		}
		if err := s.getJSON(ctx, s.kv+"/admin/migration", &st); err != nil {
			return err
		}
		if st.State == "idle" {
			return nil
		}
		s.log.Info("waiting for the drain", "copied", st.Copied, "total", st.Total)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(s.pollWait):
		}
	}
}

func (s *scaler) promValue(ctx context.Context, q string) (float64, error) {
	var a struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Value []any `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := s.getJSON(ctx, s.prom+"/api/v1/query?query="+url.QueryEscape(q), &a); err != nil {
		return 0, err
	}
	if a.Status != "success" {
		return 0, errors.New("prometheus query failed")
	}
	if len(a.Data.Result) == 0 || len(a.Data.Result[0].Value) != 2 {
		return math.NaN(), nil
	}
	str, _ := a.Data.Result[0].Value[1].(string)
	return strconv.ParseFloat(str, 64)
}

func (s *scaler) do(ctx context.Context, method, u string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return nil, err
	}
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		resp.Body.Close()
		return nil, fmt.Errorf("%s %s: %d %s", method, u, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return resp, nil
}

func (s *scaler) getJSON(ctx context.Context, u string, out any) error {
	resp, err := s.do(ctx, http.MethodGet, u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

func (s *scaler) post(ctx context.Context, u string) error {
	resp, err := s.do(ctx, http.MethodPost, u)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func round(v float64) float64 {
	if math.IsNaN(v) {
		return -1
	}
	return math.Round(v*10) / 10
}

type awsCLI struct {
	stack string
	run   func(args ...string) (string, error)
}

func (a awsCLI) stackBusy() (bool, error) {
	out, err := a.run("cloudformation", "describe-stacks", "--stack-name", a.stack, "--query", "Stacks[0].StackStatus", "--output", "text")
	if err != nil {
		return false, err
	}
	return strings.Contains(out, "IN_PROGRESS"), nil
}

func (a awsCLI) setShardCount(n int) error {
	keys, err := a.run("cloudformation", "describe-stacks", "--stack-name", a.stack, "--query", "Stacks[0].Parameters[].ParameterKey", "--output", "text")
	if err != nil {
		return err
	}
	args := []string{"cloudformation", "update-stack", "--stack-name", a.stack, "--use-previous-template", "--parameters"}
	for _, k := range strings.Fields(keys) {
		if k == "ShardCount" {
			continue
		}
		args = append(args, "ParameterKey="+k+",UsePreviousValue=true")
	}
	args = append(args, fmt.Sprintf("ParameterKey=ShardCount,ParameterValue=%d", n))
	_, err = a.run(args...)
	return err
}

func runAWS(args ...string) (string, error) {
	out, err := exec.Command("aws", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("aws %s: %v: %s", args[1], err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func main() {
	prom := flag.String("prometheus", "http://localhost:9090", "Prometheus URL")
	kv := flag.String("coordinator", "http://localhost:8080", "coordinator URL")
	stack := flag.String("stack", "capstone-app", "app stack name")
	up := flag.Duration("up", 60*time.Millisecond, "scale up when write p99 is above this")
	down := flag.Duration("down", 20*time.Millisecond, "scale down when write p99 is below this")
	upFor := flag.Duration("up-for", 2*time.Minute, "how long p99 must stay high")
	downFor := flag.Duration("down-for", 5*time.Minute, "how long p99 must stay low")
	cooldown := flag.Duration("cooldown", 5*time.Minute, "wait after an action")
	every := flag.Duration("every", 15*time.Second, "how often to check")
	minTraffic := flag.Float64("min-traffic", 1, "writes per second below which the store counts as idle")
	downRate := flag.Float64("down-rate", 100, "scale down only below this many writes per second (half of what one shard handles at the up threshold)")
	dryRun := flag.Bool("dry-run", false, "only log the decisions")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	s := &scaler{
		d: &decider{cfg: config{up: up.Seconds(), down: down.Seconds(), upFor: *upFor, downFor: *downFor,
			cooldown: *cooldown, minTraffic: *minTraffic, downRate: *downRate}},
		prom: strings.TrimRight(*prom, "/"), kv: strings.TrimRight(*kv, "/"), token: os.Getenv("KV_TOKEN"),
		drainID: "shard-2", http: &http.Client{Timeout: 10 * time.Second},
		cloud: awsCLI{stack: *stack, run: runAWS}, dryRun: *dryRun, pollWait: 5 * time.Second, log: logger,
	}
	ctx := context.Background()
	for {
		if err := s.tick(ctx, time.Now()); err != nil {
			logger.Error("tick", "error", err.Error())
		}
		time.Sleep(*every)
	}
}
