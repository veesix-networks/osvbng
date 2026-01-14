# osvbng Plugin Generator

Cookiecutter template for generating osvbng community plugins following Pattern 1.

## Prerequisites

```bash
# pipx is strongly recommended.
pipx install cookiecutter

# If pipx is not an option,
# you can install cookiecutter in your Python user directory.
python -m pip install --user cookiecutter
```

Reference https://cookiecutter.readthedocs.io/en/stable/README.html#installation on how to install cookiecutter if the above does not work for you.

## Usage

### Remote

```bash
cookiecutter gh:veesix-networks/osvbng --directory="tools/generate_plugin"
```

### Local

```bash
cookiecutter tools/generate_plugin -o plugins/community/
```

## Template Variables

- `plugin_name`: Package/folder name - single word, lowercase (e.g., `myplugin`, `wallgarden`)
- `plugin_namespace`: Dotted namespace for registration (e.g., `example.myplugin`, `community.wallgarden` NOTE: It can be the same as the plugin_name if its unique across the build)
- `plugin_description`: Brief description
- `author_name`: Author name
- `author_email`: Author email
- `version`: Plugin version

## Generated Files

```
plugins/community/{plugin_name}/
├── config.go
├── {plugin_name}.go
├── paths.go
├── status_show.go
├── message_conf.go
└── commands_cli.go
```

## Post-Generation

1. Register plugin in `plugins/community/all/{plugin_name}.go`:

```go
package all

import _ "github.com/veesix-networks/osvbng/plugins/community/{plugin_name}"
```

2. Add config to `/etc/osvbng/config.yaml`:

```yaml
plugins:
  {plugin_namespace}:
    enabled: true
    message: "Your message"
```

3. Build and test:

```bash
go build -o bin/osvbngd ./cmd/osvbngd
./bin/osvbngd -config test-infra/configs/bng-vpp.yaml
```

4. Test commands:

```bash
show {plugin_name} status
configure
set {plugin_name} message "test"
commit
```

## Documentation

- [docs/plugins/PLUGINS.md](../../docs/plugins/PLUGINS.md)
- [docs/HANDLERS.md](../../docs/HANDLERS.md)
- Example: `plugins/community/hello`
