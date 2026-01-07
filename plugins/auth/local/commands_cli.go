package local

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/veesix-networks/osvbng/cmd/osvbngcli/commands"
	"github.com/veesix-networks/osvbng/pkg/cli"
	operpaths "github.com/veesix-networks/osvbng/pkg/handlers/oper/paths"
)

func init() {
	cli.RegisterRoot(Namespace, &cli.RootCommand{
		Path:        []string{"show", "subscriber", "auth", "local"},
		Description: "Local authentication status",
	})

	cli.Register(Namespace, &cli.Command{
		Path:        []string{"show", "subscriber", "auth", "local", "users"},
		Description: "Display all local users",
		Handler:     commands.ShowHandlerFunc(ShowUsersPath),
	})

	cli.Register(Namespace, &cli.Command{
		Path:        []string{"show", "subscriber", "auth", "local", "services"},
		Description: "Display all local services",
		Handler:     commands.ShowHandlerFunc(ShowServicesPath),
	})

	cli.RegisterRoot(Namespace, &cli.RootCommand{
		Path:        []string{"exec", "subscriber", "auth", "local"},
		Description: "Local authentication operations",
	})

	cli.Register(Namespace, &cli.Command{
		Path:        []string{"exec", "subscriber", "auth", "local", "user", "create"},
		Description: "Create a new user",
		Handler:     cmdCreateUser,
		Arguments: []*cli.Argument{
			{Name: "username", Description: "Username", Type: cli.ArgUserInput},
			{Name: "password", Description: "Password (optional)", Type: cli.ArgUserInput},
		},
	})

	cli.Register(Namespace, &cli.Command{
		Path:        []string{"exec", "subscriber", "auth", "local", "user", "password"},
		Description: "Set user password",
		Handler:     cmdSetUserPassword,
		Arguments: []*cli.Argument{
			{Name: "user_id", Description: "User ID", Type: cli.ArgUserInput},
			{Name: "password", Description: "Password", Type: cli.ArgUserInput},
		},
	})

	cli.Register(Namespace, &cli.Command{
		Path:        []string{"exec", "subscriber", "auth", "local", "user", "enabled"},
		Description: "Enable or disable user",
		Handler:     cmdSetUserEnabled,
		Arguments: []*cli.Argument{
			{Name: "user_id", Description: "User ID", Type: cli.ArgUserInput},
			{Name: "enabled", Description: "Enable (true/false)", Type: cli.ArgUserInput},
		},
	})

	cli.Register(Namespace, &cli.Command{
		Path:        []string{"exec", "subscriber", "auth", "local", "user", "service"},
		Description: "Assign service to user with priority",
		Handler:     cmdSetUserService,
		Arguments: []*cli.Argument{
			{Name: "user_id", Description: "User ID", Type: cli.ArgUserInput},
			{Name: "service", Description: "Service name", Type: cli.ArgUserInput},
			{Name: "priority", Description: "Priority (lower = higher precedence)", Type: cli.ArgUserInput},
		},
	})

	cli.Register(Namespace, &cli.Command{
		Path:        []string{"exec", "subscriber", "auth", "local", "user", "attribute"},
		Description: "Set user attribute",
		Handler:     cmdSetUserAttribute,
		Arguments: []*cli.Argument{
			{Name: "username", Description: "Username", Type: cli.ArgUserInput},
			{Name: "attribute", Description: "Attribute name", Type: cli.ArgUserInput},
			{Name: "value", Description: "Attribute value", Type: cli.ArgUserInput},
			{Name: "op", Description: "Operator (=, :=, +=, etc.)", Type: cli.ArgUserInput},
		},
	})

	cli.Register(Namespace, &cli.Command{
		Path:        []string{"exec", "subscriber", "auth", "local", "service", "attribute"},
		Description: "Set service attribute",
		Handler:     cmdSetServiceAttribute,
		Arguments: []*cli.Argument{
			{Name: "service", Description: "Service name", Type: cli.ArgUserInput},
			{Name: "attribute", Description: "Attribute name", Type: cli.ArgUserInput},
			{Name: "value", Description: "Attribute value", Type: cli.ArgUserInput},
			{Name: "op", Description: "Operator (=, :=, +=, etc.)", Type: cli.ArgUserInput},
		},
	})

	cli.Register(Namespace, &cli.Command{
		Path:        []string{"exec", "subscriber", "auth", "local", "service", "create"},
		Description: "Create a new service",
		Handler:     cmdCreateService,
		Arguments: []*cli.Argument{
			{Name: "name", Description: "Service name", Type: cli.ArgUserInput},
			{Name: "description", Description: "Service description (optional)", Type: cli.ArgUserInput},
		},
	})

	cli.Register(Namespace, &cli.Command{
		Path:        []string{"exec", "subscriber", "auth", "local", "service", "delete"},
		Description: "Delete a service",
		Handler:     cmdDeleteService,
		Arguments: []*cli.Argument{
			{Name: "service", Description: "Service name", Type: cli.ArgUserInput},
		},
	})

	cli.Register(Namespace, &cli.Command{
		Path:        []string{"exec", "subscriber", "auth", "local", "user", "delete"},
		Description: "Delete a user",
		Handler:     cmdDeleteUser,
		Arguments: []*cli.Argument{
			{Name: "user_id", Description: "User ID", Type: cli.ArgUserInput},
		},
	})
}

func cmdCreateUser(ctx context.Context, c interface{}, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: exec subscriber auth local user create <username> [password]")
	}

	req := &CreateUserRequest{
		Username: args[0],
		Enabled:  true,
	}
	if len(args) >= 2 {
		req.Password = &args[1]
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to encode user request: %w", err)
	}

	return commands.ExecuteOper(ctx, c, OperCreateUserPath, string(body))
}

func cmdSetUserPassword(ctx context.Context, c interface{}, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: exec subscriber auth local user password <user_id> <password>")
	}

	req := &SetUserPasswordRequest{Password: args[1]}
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to encode request: %w", err)
	}

	path := fmt.Sprintf("subscriber.auth.local.user.%s.password", args[0])
	return commands.ExecuteOper(ctx, c, operpaths.Path(path), string(body))
}

func cmdSetUserEnabled(ctx context.Context, c interface{}, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: exec subscriber auth local user enabled <user_id> <true|false>")
	}

	req := &SetUserEnabledRequest{Enabled: args[1] == "true"}
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to encode request: %w", err)
	}

	path := fmt.Sprintf("subscriber.auth.local.user.%s.enabled", args[0])
	return commands.ExecuteOper(ctx, c, operpaths.Path(path), string(body))
}

func cmdSetUserService(ctx context.Context, c interface{}, args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: exec subscriber auth local user service <user_id> <service> <priority>")
	}

	priority := 0
	fmt.Sscanf(args[2], "%d", &priority)
	req := &SetUserServiceRequest{Priority: priority}
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to encode request: %w", err)
	}

	path := fmt.Sprintf("subscriber.auth.local.user.%s.service.%s", args[0], args[1])
	return commands.ExecuteOper(ctx, c, operpaths.Path(path), string(body))
}

func cmdSetUserAttribute(ctx context.Context, c interface{}, args []string) error {
	if len(args) < 4 {
		return fmt.Errorf("usage: exec subscriber auth local user attribute <user_id> <attribute> <value> <op>")
	}

	req := &SetUserAttributeRequest{
		Attribute: args[1],
		Value:     args[2],
		Op:        args[3],
	}
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to encode request: %w", err)
	}

	path := fmt.Sprintf("subscriber.auth.local.user.%s.attribute", args[0])
	return commands.ExecuteOper(ctx, c, operpaths.Path(path), string(body))
}

func cmdSetServiceAttribute(ctx context.Context, c interface{}, args []string) error {
	if len(args) < 4 {
		return fmt.Errorf("usage: exec subscriber auth local service attribute <service> <attribute> <value> <op>")
	}

	req := &SetServiceAttributeRequest{
		Attribute: args[1],
		Value:     args[2],
		Op:        args[3],
	}
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to encode request: %w", err)
	}

	path := fmt.Sprintf("subscriber.auth.local.services.%s.attribute", args[0])
	return commands.ExecuteOper(ctx, c, operpaths.Path(path), string(body))
}

func cmdCreateService(ctx context.Context, c interface{}, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: exec subscriber auth local service create <name> [description]")
	}

	req := &CreateServiceRequest{
		Name: args[0],
	}
	if len(args) >= 2 {
		req.Description = args[1]
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to encode service request: %w", err)
	}

	return commands.ExecuteOper(ctx, c, OperCreateServicePath, string(body))
}

func cmdDeleteService(ctx context.Context, c interface{}, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: exec subscriber auth local service delete <service>")
	}

	path := fmt.Sprintf("subscriber.auth.local.services.%s.delete", args[0])
	return commands.ExecuteOper(ctx, c, operpaths.Path(path), "{}")
}

func cmdDeleteUser(ctx context.Context, c interface{}, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: exec subscriber auth local user delete <user_id>")
	}

	path := fmt.Sprintf("subscriber.auth.local.user.%s.delete", args[0])
	return commands.ExecuteOper(ctx, c, operpaths.Path(path), "{}")
}
