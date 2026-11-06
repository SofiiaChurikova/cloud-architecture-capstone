# Spec Delta

## Purpose

The API of the store. Clients talk only to the coordinator and don't know about shards.

## ADDED Requirements

### Requirement: Table can be created
The API SHALL let a client create a table with a name and a partition key name.

#### Scenario: Create a table
- **WHEN** a client sends `PUT /tables/users` with `{"partition_key": "id"}`
- **THEN** the table is created and `GET /tables/users` returns it

### Requirement: Item can be created
The API SHALL store a JSON item under its key.

#### Scenario: Put an item
- **WHEN** a client sends `PUT /tables/users/items/42` with a JSON body
- **THEN** the API returns `200` and the item is stored

### Requirement: Item can be read
The API SHALL return the stored item for a key.

#### Scenario: Read an item
- **WHEN** a client sends `GET /tables/users/items/42`
- **THEN** the API returns the item, or `404` if there is no such key

### Requirement: Item existence can be checked
The API SHALL say if an item exists without returning it.

#### Scenario: Check an item
- **WHEN** a client sends `HEAD /tables/users/items/42`
- **THEN** the API returns `200` if it exists and `404` if not

### Requirement: Item can be deleted
The API SHALL delete the item for a key.

#### Scenario: Delete an item
- **WHEN** a client deletes `/tables/users/items/42` and then reads it
- **THEN** the read returns `404`
