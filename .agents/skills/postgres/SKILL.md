---
name: postgres
description: PostgreSQL best practices, query optimization, connection troubleshooting, and performance improvement.
license: MIT
metadata:
  author: planetscale
  version: "1.0.0"
---

# Postgres

## Generic Postgres

| Topic                  | Reference                                                        | Use for                                                   |
| ---------------------- | ---------------------------------------------------------------- | --------------------------------------------------------- |
| Schema Design          | [references/schema-design.md](https://raw.githubusercontent.com/planetscale/database-skills/main/skills/postgres/references/schema-design.md)           | Tables, primary keys, data types, foreign keys            |
| Indexing               | [references/indexing.md](https://raw.githubusercontent.com/planetscale/database-skills/main/skills/postgres/references/indexing.md)                      | Index types, composite indexes, performance               |
| Index Optimization     | [references/index-optimization.md](https://raw.githubusercontent.com/planetscale/database-skills/main/skills/postgres/references/index-optimization.md) | Unused/duplicate index queries, index audit               |
| Partitioning           | [references/partitioning.md](https://raw.githubusercontent.com/planetscale/database-skills/main/skills/postgres/references/partitioning.md)             | Large tables, time-series, data retention                 |
| Query Patterns         | [references/query-patterns.md](https://raw.githubusercontent.com/planetscale/database-skills/main/skills/postgres/references/query-patterns.md)         | SQL anti-patterns, JOINs, pagination, batch queries       |
| Optimization Checklist | [references/optimization-checklist.md](https://raw.githubusercontent.com/planetscale/database-skills/main/skills/postgres/references/optimization-checklist.md) | Pre-optimization audit, cleanup, readiness checks  |
| MVCC and VACUUM        | [references/mvcc-vacuum.md](https://raw.githubusercontent.com/planetscale/database-skills/main/skills/postgres/references/mvcc-vacuum.md)               | Dead tuples, long transactions, xid wraparound prevention |

## Operations and Architecture

| Topic                  | Reference                                                                    | Use for                                                         |
| ---------------------- | ---------------------------------------------------------------------------- | --------------------------------------------------------------- |
| MVCC Transactions      | [references/mvcc-transactions.md](https://raw.githubusercontent.com/planetscale/database-skills/main/skills/postgres/references/mvcc-transactions.md)           | Isolation levels, XID wraparound, serialization errors          |
| WAL and Checkpoints    | [references/wal-operations.md](https://raw.githubusercontent.com/planetscale/database-skills/main/skills/postgres/references/wal-operations.md)                 | WAL internals, checkpoint tuning, durability, crash recovery    |
| Monitoring             | [references/monitoring.md](https://raw.githubusercontent.com/planetscale/database-skills/main/skills/postgres/references/monitoring.md)                         | pg_stat views, logging, pg_stat_statements, host metrics        |