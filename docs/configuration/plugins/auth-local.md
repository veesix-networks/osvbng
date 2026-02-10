# subscriber.auth.local

Local subscriber authentication using a SQLite database.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `database_path` | string | Path to the SQLite subscriber database | `/var/lib/osvbng/subscribers.db` |
| `allow_all` | bool | Allow all subscribers without checking the database | `false` |

## Example

```yaml
plugins:
  subscriber.auth.local:
    database_path: /var/lib/osvbng/subscribers.db
    allow_all: false
```
