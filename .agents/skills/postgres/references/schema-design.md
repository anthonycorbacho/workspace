---
title: PostgreSQL Schema Design
description: Schema design guide
tags: postgres, schema, primary-keys, data-types, foreign-keys, naming
---

# Schema Design

## Primary Keys

Prefer `TEXT`. Avoid random UUIDs (UUIDv4) as primary keys; use `uuidv7()` when you need UUIDs.

```sql
CREATE TABLE domain_users (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE
);
```

Random UUID PKs (v4) can cause index fragmentation; UUIDs are also larger (16 vs 8 bytes for BIGINT) and can slow joins.

## Data Types

| Use | Avoid |
| --- | --- |
| `TEXT`, `VARCHAR` | Extension-specific types |
| `JSONB` | Custom ENUMs (use CHECK instead) |
| `TIMESTAMPTZ` | `TIMESTAMP` without time zone |
| `BIGINT`, `INTEGER` | Platform-specific types |

Prefer CHECK constraints over ENUM types — they're easier to modify:

```sql
CREATE TABLE domain_orders (
  id TEXT PRIMARY KEY,
  status TEXT NOT NULL CHECK (status IN ('pending', 'shipped', 'delivered'))
);
```

## Foreign Keys

- Always index FK columns (PostgreSQL does not auto-create these)
- Avoid circular FK dependencies
- Suggestion: use `ON DELETE CASCADE` or `ON DELETE SET NULL` explicitly when on the same domain

```sql
CREATE TABLE domain_orders (
  id TEXT PRIMARY KEY,
  customer_id TEXT NOT NULL REFERENCES domain_customers(id) ON DELETE CASCADE
);
CREATE INDEX domain_orders_customer_id_idx ON domain_orders (customer_id);
```

## Naming Conventions

- Tables: plurals snake_case with domain (`iam_users`, `external_resources`) that follow the repository structure
- Columns: singular snake_case (`create_tile`, `user_id`)
- Indexes: `{domain}_{table}_{column}_idx`
- Constraints: `{domain}_{table}_{column}_{type}` (e.g., `order_status_check`)

## General Guidelines

- Add `NOT NULL` to as many columns as possible
- Add `create_time TIMESTAMPTZ DEFAULT NOW()` to all tables
- Use `TEXT` for all IDs and foreign keys, even on small tables
- Keep tables normalized; denormalize only for proven hot read paths
