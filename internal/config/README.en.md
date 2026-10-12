# internal/config: Configuration Loader & Validation

[中文](README.md) | [English](README.en.md)

`internal/config` manages configuration parsing, schema validation, and environment variable overrides for Ora Cloud.

## Responsibilities

- **Structured configuration**: Defines strongly-typed Go structs mapping the entire system configuration:
  - `ServerConfig`: Port, Gin mode, read/write timeouts.
  - `LoggerConfig`: Log level, file paths, rotation thresholds (max size, age, backups, gzip compression).
  - `DatabaseConfig`: Driver (must be `postgres`), DSN, and connection pool limits (`max_open_conns`, `max_idle_conns`, `conn_max_lifetime`).
  - `AuthConfig`: Expected token audience and list of `TrustedKey` verification parameters.
- **Hierarchical loading via Viper**:
  - Searches for `config.yaml` in `./configs`, `../configs`, and `.`.
  - Supports explicit file path overriding via `-config <path>`.
  - Maps environment variables with the `CLOUD_` prefix, replacing dots with underscores (e.g., `CLOUD_DATABASE_DSN` overrides `database.dsn`).
- **Startup sanity validation**: Rejects invalid configurations with explicit errors, requiring positive durations for `read_timeout`, `write_timeout`, and `conn_max_lifetime`.

## Boundaries and invariants

The startup settlement proposal uses `issue_runs.agent_start_timeout`, defaulting to 30 minutes. A positive duration can be supplied through `CLOUD_ISSUE_RUNS_AGENT_START_TIMEOUT`; an absent, zero or negative value selects the default. The database clock owns expiry and additional input does not renew it. Registered executions retain their Workspace and recovery evidence until a real terminal result arrives. See [Startup settlement](../../docs/agent-startup-settlement.en.md) for semantics and the review boundary.

The optional `object_store` section configures S3 delivery. When enabled, the server loads separately mounted access/secret key files into memory and validates private/public endpoints, region, bucket, addressing mode and a TTL between one second and seven days. All keys support `CLOUD_OBJECT_STORE_*` overrides. Short-lived PUT grants can bind SHA-256; Cloud verifies existence, size and stored checksum through private HEAD before registering Revision. Disabled by default, delivery settles skipped/object_store_unconfigured through the caller's transaction without dispatch; standalone grant requests return unavailable. Health reports configured/unconfigured, not S3 availability.

- **No secret storage**: Configuration files store only public verification keys and infrastructure references. Plaintext deployment secrets and private keys must never appear in configuration files.
- **Immutable runtime**: Configurations are loaded once at command startup and passed as ready-to-use values. There is no global mutable configuration singleton.

See [config.yaml](../../configs/config.yaml), [cmd/server](../../cmd/server/README.en.md), and [Authentication](../../docs/authentication.md).
