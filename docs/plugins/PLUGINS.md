# Plugins

A plugin allows you to add, replace or extend functionality within osvbng. They are divided up into directories within the `plugins/` folder. Please ensure you read the summary table below to determine where your plugin should live.

| Directory | What should be here? |
| ---------------- | -------------------- |
| `plugins/auth`     | Authentication plugins (providers) |
| `plugins/cache`    | Cache plugins (providers) |
| `plugins/community` | Typically most plugins should live here, if you extend or add new functionality then it should live here |
| `plugins/dhcp4`     | DHCPv4 based plugins (eg. proxy and relay implementations, or your own server implementation) |
| `plugins/exporter`  | Plugins that expose the internal state/metrics eg. Prometheus Exporter, SNMP, etc.. |

Plugins in any directory other than the `community` are designed to be maintained by core developers and are typically what we refer as `certified` plugins.

### Expectations

Issues and PRs raised for the codebase for `plugins/community` have a low priority unless the maintainer can contribute to the fix, all core/non community plugins are a priority so please ensure you understand this if you are contributing to the osvbng project.

## Component vs Provider

In short, a component typically has its own lifecycle and is independent of the core implementation (but may import things from the core codebase) whereas providers swap out implementation that already exist in the core, essentially changing the behaviour at specific points of the component itself. An authentication plugin is typically a provider that implements the AuthenticationProvider interface, but a new feature like CGN or wallgarden would be a separate component.

For more details please [refer to the README.md](../README.md)

## Folder/File Structure

There are 2 patterns to implement a plugin. Use **Pattern 1** unless you understand exactly why you should use **Pattern 2**.

### Pattern 1 - Simple

All files live in the plugin root directory:

| Path | Purpose |
| ---- | ------- |
| `plugins/community/{project}/config.go` | Namespace constant, config struct, and registration |
| `plugins/community/{project}/{project}.go` | Component implementation with Start/Stop lifecycle |
| `plugins/community/{project}/paths.go` | Path constants for show/conf handlers |
| `plugins/community/{project}/status_show.go` | Show command handler implementation |
| `plugins/community/{project}/message_conf.go` | Config command handler implementation |
| `plugins/community/{project}/commands_cli.go` | CLI command registration |

### Pattern 2 - Advanced

Files organized into subdirectories for better maintainability:

| Path | Purpose |
| ---- | ------- |
| `plugins/community/{project}/config.go` | Namespace constant, config struct, and registration |
| `plugins/community/{project}/{project}.go` | Component implementation with Start/Stop lifecycle |
| `plugins/community/{project}/paths.go` | Path constants for show/conf handlers |
| `plugins/community/{project}/show/status.go` | Show command handler (package: `show`) |
| `plugins/community/{project}/conf/message.go` | Config command handler (package: `conf`) |
| `plugins/community/{project}/commands_cli.go` | CLI command registration |

**Important**: For Pattern 2, subdirectory files use separate packages (`show`, `conf`) and must import the parent `{project}` package to access shared types.

## Quick Start Example

See `plugins/community/hello` for a complete working example following Pattern 2.

## Implementation Guide

### 1. config.go - Registration and Configuration

This file contains your plugin namespace, config struct, and all registration:

```go
package myplugin

import (
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/conf"
)

const Namespace = "example.myplugin"

type Config struct {
	Enabled bool   `json:"enabled" yaml:"enabled"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}

func init() {
	// Register plugin config type
	conf.RegisterPluginConfig(Namespace, Config{})

	// Register component factory
	component.Register(Namespace, NewComponent,
		component.WithAuthor("Your Name"),
		component.WithVersion("1.0.0"),
	)
}
```

### 2. {project}.go - Component Implementation

Define your component struct and implement the required lifecycle methods:

```go
package myplugin

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/conf"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

type Component struct {
	*component.Base
	logger  *slog.Logger
	message string
}

func NewComponent(deps component.Dependencies) (component.Component, error) {
	// Get typed config from registry
	pluginCfgRaw, ok := conf.GetPluginConfig(Namespace)
	if !ok {
		return nil, nil // Config not present
	}

	pluginCfg, ok := pluginCfgRaw.(*Config)
	if !ok {
		return nil, fmt.Errorf("invalid config type for %s", Namespace)
	}

	if !pluginCfg.Enabled {
		return nil, nil // Plugin disabled
	}

	return &Component{
		Base:    component.NewBase(Namespace),
		logger:  logger.Component(Namespace),
		message: pluginCfg.Message,
	}, nil
}

func (c *Component) Start(ctx context.Context) error {
	c.StartContext(ctx)
	c.logger.Info("Plugin started", "message", c.message)
	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.StopContext()
	c.logger.Info("Plugin stopped")
	return nil
}

func (c *Component) GetMessage() string {
	return c.message
}

func (c *Component) SetMessage(msg string) {
	c.message = msg
}
```

### 3. paths.go - Path Constants

Define all your show, config, and state paths in one place:

```go
package myplugin

import (
	confpaths "github.com/veesix-networks/osvbng/pkg/conf/paths"
	showpaths "github.com/veesix-networks/osvbng/pkg/show/paths"
	statepaths "github.com/veesix-networks/osvbng/pkg/state/paths"
)

const (
	ShowStatusPath  = showpaths.Path("example.myplugin.status")
	StateStatusPath = statepaths.Path("example.myplugin.status")
	ConfMessagePath = confpaths.Path("plugins.example.myplugin.message")
)
```

The `StateStatusPath` is used for collector registration to enable periodic caching of your plugin's metrics.

### 4. Show Handler (Pattern 1: status_show.go, Pattern 2: show/status.go)

Show handlers collect and display system state/information. For detailed documentation on the handler interface and methods (`Collect`, `PathPattern`, `Dependencies`), see [HANDLERS.md](../HANDLERS.md#show-handlers).

**Pattern 1** (same package):
```go
package myplugin

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/show/handlers"
	"github.com/veesix-networks/osvbng/pkg/show/paths"
)

func init() {
	handlers.RegisterFactory(NewStatusHandler)
}

type StatusHandler struct {
	deps *handlers.ShowDeps
}

type Status struct {
	Message string `json:"message"`
	Enabled bool   `json:"enabled"`
}

func NewStatusHandler(deps *handlers.ShowDeps) handlers.ShowHandler {
	return &StatusHandler{deps: deps}
}

func (h *StatusHandler) Collect(ctx context.Context, req *handlers.ShowRequest) (interface{}, error) {
	message := "Default message"
	enabled := false

	if comp, ok := h.deps.PluginComponents[Namespace]; ok {
		if myComp, ok := comp.(*Component); ok {
			message = myComp.GetMessage()
			enabled = true
		}
	}

	return &Status{
		Message: message,
		Enabled: enabled,
	}, nil
}

func (h *StatusHandler) PathPattern() paths.Path {
	return paths.Path(ShowStatusPath)
}

func (h *StatusHandler) Dependencies() []paths.Path {
	return nil
}
```

**Pattern 2** (separate package - imports parent):
```go
package show

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/show/handlers"
	"github.com/veesix-networks/osvbng/pkg/show/paths"
	"github.com/veesix-networks/osvbng/pkg/state"
	"github.com/veesix-networks/osvbng/plugins/community/myplugin"
)

func init() {
	handlers.RegisterFactory(NewStatusHandler)

	// Register metric collector to periodically cache this data for exporters
	state.RegisterMetric(myplugin.StateStatusPath, myplugin.ShowStatusPath)
}

type StatusHandler struct {
	deps *handlers.ShowDeps
}

type Status struct {
	Message string `json:"message"`
	Enabled bool   `json:"enabled"`
}

func NewStatusHandler(deps *handlers.ShowDeps) handlers.ShowHandler {
	return &StatusHandler{deps: deps}
}

func (h *StatusHandler) Collect(ctx context.Context, req *handlers.ShowRequest) (interface{}, error) {
	message := "Default message"
	enabled := false

	if comp, ok := h.deps.PluginComponents[myplugin.Namespace]; ok {
		if myComp, ok := comp.(*myplugin.Component); ok {
			message = myComp.GetMessage()
			enabled = true
		}
	}

	return &Status{
		Message: message,
		Enabled: enabled,
	}, nil
}

func (h *StatusHandler) PathPattern() paths.Path {
	return paths.Path(myplugin.ShowStatusPath)
}

func (h *StatusHandler) Dependencies() []paths.Path {
	return nil
}
```

### 5. Config Handler (Pattern 1: message_conf.go, Pattern 2: conf/message.go)

Config handlers validate and apply configuration changes. For detailed documentation on the handler interface and methods (`Validate`, `Apply`, `Rollback`, `PathPattern`, `Dependencies`, `Callbacks`), see [HANDLERS.md](../HANDLERS.md#config-handlers).

**Pattern 1** (same package):
```go
package myplugin

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/veesix-networks/osvbng/pkg/conf/handlers"
	"github.com/veesix-networks/osvbng/pkg/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

func init() {
	handlers.RegisterFactory(NewMessageHandler)
}

type MessageHandler struct {
	deps   *handlers.ConfDeps
	logger *slog.Logger
}

func NewMessageHandler(deps *handlers.ConfDeps) handlers.Handler {
	return &MessageHandler{
		deps:   deps,
		logger: logger.Component("myplugin.conf"),
	}
}

func (h *MessageHandler) Validate(ctx context.Context, hctx *handlers.HandlerContext) error {
	message, ok := hctx.NewValue.(string)
	if !ok {
		return fmt.Errorf("message must be a string")
	}
	if message == "" {
		return fmt.Errorf("message cannot be empty")
	}
	return nil
}

func (h *MessageHandler) Apply(ctx context.Context, hctx *handlers.HandlerContext) error {
	message := hctx.NewValue.(string)

	comp, ok := h.deps.PluginComponents[Namespace]
	if !ok {
		return fmt.Errorf("%s component not loaded", Namespace)
	}

	myComp, ok := comp.(*Component)
	if !ok {
		return fmt.Errorf("invalid component type for %s", Namespace)
	}

	myComp.SetMessage(message)
	h.logger.Info("Updated message", "message", message)
	return nil
}

func (h *MessageHandler) Rollback(ctx context.Context, hctx *handlers.HandlerContext) error {
	if hctx.OldValue == nil {
		return nil
	}

	oldMessage := hctx.OldValue.(string)
	comp, ok := h.deps.PluginComponents[Namespace]
	if !ok {
		return nil
	}

	myComp, ok := comp.(*Component)
	if !ok {
		return nil
	}

	myComp.SetMessage(oldMessage)
	return nil
}

func (h *MessageHandler) PathPattern() paths.Path {
	return paths.Path(ConfMessagePath)
}

func (h *MessageHandler) Dependencies() []paths.Path {
	return nil
}

func (h *MessageHandler) Callbacks() *handlers.Callbacks {
	return nil
}
```

**Pattern 2** (separate package - imports parent):
```go
package conf

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/veesix-networks/osvbng/pkg/conf/handlers"
	"github.com/veesix-networks/osvbng/pkg/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/plugins/community/myplugin"
)

func init() {
	handlers.RegisterFactory(NewMessageHandler)
}

type MessageHandler struct {
	deps   *handlers.ConfDeps
	logger *slog.Logger
}

func NewMessageHandler(deps *handlers.ConfDeps) handlers.Handler {
	return &MessageHandler{
		deps:   deps,
		logger: logger.Component("myplugin.conf"),
	}
}

func (h *MessageHandler) Validate(ctx context.Context, hctx *handlers.HandlerContext) error {
	message, ok := hctx.NewValue.(string)
	if !ok {
		return fmt.Errorf("message must be a string")
	}
	if message == "" {
		return fmt.Errorf("message cannot be empty")
	}
	return nil
}

func (h *MessageHandler) Apply(ctx context.Context, hctx *handlers.HandlerContext) error {
	message := hctx.NewValue.(string)

	comp, ok := h.deps.PluginComponents[myplugin.Namespace]
	if !ok {
		return fmt.Errorf("%s component not loaded", myplugin.Namespace)
	}

	myComp, ok := comp.(*myplugin.Component)
	if !ok {
		return fmt.Errorf("invalid component type for %s", myplugin.Namespace)
	}

	myComp.SetMessage(message)
	h.logger.Info("Updated message", "message", message)
	return nil
}

func (h *MessageHandler) Rollback(ctx context.Context, hctx *handlers.HandlerContext) error {
	if hctx.OldValue == nil {
		return nil
	}

	oldMessage := hctx.OldValue.(string)
	comp, ok := h.deps.PluginComponents[myplugin.Namespace]
	if !ok {
		return nil
	}

	myComp, ok := comp.(*myplugin.Component)
	if !ok {
		return nil
	}

	myComp.SetMessage(oldMessage)
	return nil
}

func (h *MessageHandler) PathPattern() paths.Path {
	return paths.Path(myplugin.ConfMessagePath)
}

func (h *MessageHandler) Dependencies() []paths.Path {
	return nil
}

func (h *MessageHandler) Callbacks() *handlers.Callbacks {
	return nil
}
```

### 6. commands_cli.go - CLI Command Registration

There are three ways to define CLI command handlers, depending on your needs:

#### Method 1: Simple Show Commands (Recommended)

Use `commands.ShowHandlerFunc(path)` for most show commands. The CLI framework automatically validates required arguments based on the `Arguments` field:

```go
package myplugin

import (
	"github.com/veesix-networks/osvbng/cmd/osvbngcli/commands"
	"github.com/veesix-networks/osvbng/pkg/cli"
)

func init() {
	cli.RegisterRoot(Namespace, &cli.RootCommand{
		Path:        []string{"show", "myplugin"},
		Description: "My plugin commands",
	})

	// Command with no arguments
	cli.Register(Namespace, &cli.Command{
		Path:        []string{"show", "myplugin", "status"},
		Description: "Display plugin status",
		Handler:     commands.ShowHandlerFunc(ShowStatusPath),
	})

	// Command with required argument (automatic validation)
	cli.Register(Namespace, &cli.Command{
		Path:        []string{"show", "myplugin", "session"},
		Description: "Display session details",
		Handler:     commands.ShowHandlerFunc(ShowSessionPath),
		Arguments: []*cli.Argument{
			{Name: "session-id", Description: "Session identifier", Type: cli.ArgUserInput},
		},
	})
}
```

#### Method 2: Show Commands with Custom Validation

Use `commands.ShowHandlerFuncWithValidator()` only when you need custom validation logic beyond checking if required arguments are present:

```go
package myplugin

import (
	"fmt"

	"github.com/veesix-networks/osvbng/cmd/osvbngcli/commands"
	"github.com/veesix-networks/osvbng/pkg/cli"
)

func init() {
	cli.RegisterRoot(Namespace, &cli.RootCommand{
		Path:        []string{"show", "myplugin"},
		Description: "My plugin commands",
	})

	// Custom validator to check argument value
	cli.Register(Namespace, &cli.Command{
		Path:        []string{"show", "myplugin", "info"},
		Description: "Display plugin info",
		Handler:     commands.ShowHandlerFuncWithValidator(
			ShowInfoPath,
			func(args []string) error {
				if len(args) > 0 && args[0] != "verbose" && args[0] != "brief" {
					return fmt.Errorf("mode must be 'verbose' or 'brief'")
				}
				return nil
			},
		),
		Arguments: []*cli.Argument{
			{Name: "mode", Description: "Display mode (verbose|brief)", Type: cli.ArgUserInput},
		},
	})
}
```

#### Method 3: Custom Handler Functions

Use custom handler functions for complex logic or config commands:

```go
package myplugin

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/cmd/osvbngcli/commands"
	"github.com/veesix-networks/osvbng/pkg/cli"
)

func init() {
	cli.RegisterRoot(Namespace, &cli.RootCommand{
		Path:        []string{"set", "myplugin"},
		Description: "Configure my plugin",
	})

	cli.Register(Namespace, &cli.Command{
		Path:        []string{"set", "myplugin", "message"},
		Description: "Set the message",
		Handler:     cmdSetMessage,
		Arguments: []*cli.Argument{
			{Name: "text", Description: "Message text", Type: cli.ArgUserInput},
		},
	})
}

func cmdSetMessage(ctx context.Context, c interface{}, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: set myplugin message <text>")
	}

	// Custom logic here - e.g., transform the input
	message := args[0]

	return commands.ExecuteConfigSet(ctx, c, ConfMessagePath, message)
}
```

**Summary:**
- **Method 1**: Use for most show commands (automatic validation of required arguments)
- **Method 2**: Use for show commands that need custom validation logic (e.g., checking argument values)
- **Method 3**: Use for config commands or when you need custom pre-processing logic before executing the handler

**Note:** The CLI framework automatically validates that required arguments (defined with `Type: cli.ArgUserInput`) are provided, so you don't need Method 2 just for basic presence checks.

### 7. Register Plugin in plugins/community/all/

Create `plugins/community/all/myplugin.go`:

**Pattern 1**:
```go
package all

import _ "github.com/veesix-networks/osvbng/plugins/community/myplugin"
```

**Pattern 2**:
```go
package all

import (
	_ "github.com/veesix-networks/osvbng/plugins/community/myplugin"
	_ "github.com/veesix-networks/osvbng/plugins/community/myplugin/conf"
	_ "github.com/veesix-networks/osvbng/plugins/community/myplugin/show"
)
```

### 8. Configuration File

Add your plugin config to `/etc/osvbng/config.yaml`:

```yaml
plugins:
  example.myplugin:
    enabled: true
    message: "Hello from my plugin"
```

## Exposing Metrics for Exporters

If you want your plugin's data to be periodically cached for consumption by exporters (Prometheus, SNMP, etc.), register a collector in your show handler's `init()` function:

```go
import "github.com/veesix-networks/osvbng/pkg/state"

func init() {
    handlers.RegisterFactory(NewStatusHandler)

    // Enable periodic caching for exporters
    state.RegisterMetric(myplugin.StateStatusPath, myplugin.ShowStatusPath)
}
```

This wraps your show handler in a collector that periodically (default: every 5 seconds) calls the handler and caches the result. Exporters read from cache instead of calling components directly.

**Important:**
- Collectors run by default for all registered metrics
- To disable a specific collector, add it to `monitoring.disabled_collectors` in config
- The CLI and gRPC API call show handlers directly for real-time data
- Collectors are only for exporters

See [COLLECTORS.md](../COLLECTORS.md) for details.

## Key Concepts

### Package Structure (Pattern 2)

For Pattern 2, subdirectories use **separate packages** that import the parent:

- `myplugin/config.go` - package `myplugin`
- `myplugin/myplugin.go` - package `myplugin`
- `myplugin/paths.go` - package `myplugin`
- `myplugin/commands_cli.go` - package `myplugin`
- `myplugin/conf/message.go` - package `conf` (imports `myplugin`)
- `myplugin/show/status.go` - package `show` (imports `myplugin`)

This allows subdirectories to access shared types (`Namespace`, `Component`, path constants) from the parent package.

### Config Type Registration

The plugin config system uses a typed registry:

1. Define your config struct in `config.go`
2. Register it with `conf.RegisterPluginConfig(Namespace, Config{})`
3. Access it in `New()` with `conf.GetPluginConfig(Namespace)`
4. Type assert to your struct: `pluginCfg.(*Config)`

The config is automatically loaded from YAML and saved with commits.

### Component Lifecycle

Components must implement:
- `Start(ctx context.Context) error` - Initialize resources
- `Stop(ctx context.Context) error` - Clean up resources

Use `c.StartContext(ctx)` and `c.StopContext()` from `component.Base` to manage lifecycle.

### Path Constants

Define all paths once in `paths.go` and reference them everywhere:
- Show handlers: `ShowStatusPath`
- Config handlers: `ConfMessagePath`
- CLI commands: Both show and config paths

This ensures consistency and makes refactoring easier.

## Testing Your Plugin

1. Build: `go build -o bin/osvbngd ./cmd/osvbngd`
2. Run: `./bin/osvbngd -config test-infra/configs/bng-vpp.yaml`
3. Use CLI to test commands:
   - `show myplugin status`
   - `configure` then `set myplugin message "test"`
   - `commit` to persist changes

## Reference

- Example plugin: `plugins/community/hello`
- Handler documentation: [HANDLERS.md](../HANDLERS.md)
- Component interface: `pkg/component/component.go`
- Config system: `pkg/conf/`
- Show handlers: `pkg/show/handlers/`
- Conf handlers: `pkg/conf/handlers/`
