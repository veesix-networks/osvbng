package main

func RegisterCommands(tree *CommandTree) {
	tree.AddCommand([]string{"show", "subscriber", "sessions"},
		"Display subscriber sessions",
		cmdShowSubscriberSessions,
	)

	tree.AddCommand([]string{"show", "subscriber", "session"},
		"Display subscriber session details",
		cmdShowSubscriberSession,
		&Argument{Name: "session-id", Description: "Session identifier", Type: ArgUserInput},
	)

	tree.AddCommand([]string{"show", "subscriber", "stats"},
		"Display subscriber statistics",
		cmdShowSubscriberStats,
	)

	tree.AddCommand([]string{"show", "vrfs"},
		"Display virtual routing and forwarding instances",
		cmdShowVRFs,
	)

	tree.AddCommand([]string{"show", "aaa", "radius", "servers"},
		"Display RADIUS server statistics",
		cmdShowAAARadiusServers,
	)

	tree.AddCommand([]string{"show", "protocols", "bgp", "statistics"},
		"Display IPv4 BGP statistics",
		cmdShowProtocolsBGPStatistics,
	)

	tree.AddCommand([]string{"show", "system", "threads"},
		"Display Threading Information",
		cmdShowSystemThreads,
	)

	tree.AddCommand([]string{"show", "protocols", "bgp", "ipv6", "statistics"},
		"Display IPv6 BGP statistics",
		cmdShowProtocolsBGPIPv6Statistics,
	)

	tree.AddCommand([]string{"show", "running-config"},
		"Display running configuration",
		cmdShowRunningConfig,
	)

	tree.AddCommand([]string{"show", "startup-config"},
		"Display startup configuration",
		cmdShowStartupConfig,
	)

	tree.AddCommand([]string{"show", "config", "history"},
		"Display configuration version history",
		cmdShowConfigHistory,
	)

	tree.AddCommand([]string{"show", "config", "version"},
		"Display specific configuration version",
		cmdShowConfigVersion,
		&Argument{Name: "version", Description: "Version number", Type: ArgUserInput},
	)

	tree.AddCommand([]string{"configure"},
		"Enter configuration mode",
		cmdConfigure,
	)

	tree.AddCommand([]string{"set"},
		"Set configuration value",
		cmdSet,
		&Argument{Name: "path", Description: "Configuration path", Type: ArgUserInput},
		&Argument{Name: "value", Description: "Value to set", Type: ArgUserInput},
	)

	tree.AddCommand([]string{"commit"},
		"Commit configuration changes",
		cmdCommit,
		&Argument{Name: "message", Description: "Commit message", Type: ArgUserInput},
	)

	tree.AddCommand([]string{"discard"},
		"Discard configuration changes",
		cmdDiscard,
	)

	tree.AddCommand([]string{"compare"},
		"Show configuration changes",
		cmdCompare,
	)

	tree.AddCommand([]string{"exit"},
		"Exit configuration mode",
		cmdExitConfig,
	)

	tree.AddCommand([]string{"session", "terminate"},
		"Terminate a session",
		cmdSessionTerminate,
		&Argument{Name: "session-id", Description: "Session identifier", Type: ArgUserInput},
	)

	tree.AddCommand([]string{"clear", "screen"},
		"Clear the screen",
		cmdClearScreen,
	)

	tree.AddCommand([]string{"help"},
		"Display available commands",
		cmdHelp,
	)

	tree.AddCommand([]string{"show", "interface"},
		"Display interface information",
		VPPCommand("show", "interface"),
		&Argument{Name: "name", Description: "Interface name", Type: ArgUserInput},
	)

	tree.AddCommand([]string{"show", "interfaces"},
		"Display all Interfaces",
		VPPCommand("show", "interface"),
	)

	tree.AddCommand([]string{"show", "ip", "table"},
		"Display IP routing tables/VRFs",
		cmdShowIPTable,
	)

	tree.AddCommand([]string{"show", "ip", "route"},
		"Display Routing Table",
		VPPCommand("show", "ip", "fib"),
	)

	tree.AddCommand([]string{"show", "ip", "adjacency"},
		"Display Adjacency Table",
		VPPCommand("show", "adj"),
	)

	tree.AddDevCommand([]string{"vppctl"},
		"Execute VPP command (dev mode only)",
		cmdVppctl,
	)
}
