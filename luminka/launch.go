// FILE: luminka/launch.go
// PURPOSE: Parse runtime launch flags into runtime override options.
// OWNS: Root policy constants and launch flag parsing for runtime startup.
// EXPORTS: RootPolicy, RootPolicyPortable, RootPolicyDetached
// DOCS: agent_chat/plan_luminka_stream_runtime_2026-04-01.md

package luminka

import (
	"fmt"
)

type RootPolicy string

const (
	RootPolicyPortable RootPolicy = "portable"
	RootPolicyDetached RootPolicy = "detached"
)

type launchOptions struct {
	Root       string
	RootPolicy RootPolicy
	Headless   bool
}

func parseLaunchOptions(args []string) (launchOptions, error) {
	var opts launchOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--root":
			value, next, err := launchFlagValue(args, i, "--root")
			if err != nil {
				return launchOptions{}, err
			}
			opts.Root = value
			i = next
		case "--root-policy":
			value, next, err := launchFlagValue(args, i, "--root-policy")
			if err != nil {
				return launchOptions{}, err
			}
			policy, err := parseRootPolicyValue(value)
			if err != nil {
				return launchOptions{}, err
			}
			if err := mergeLaunchPolicy(&opts, policy); err != nil {
				return launchOptions{}, err
			}
			i = next
		case "--portable":
			if err := mergeLaunchPolicy(&opts, RootPolicyPortable); err != nil {
				return launchOptions{}, err
			}
		case "--detached":
			if err := mergeLaunchPolicy(&opts, RootPolicyDetached); err != nil {
				return launchOptions{}, err
			}
		case "--headless":
			opts.Headless = true
		}
	}
	return opts, nil
}

func launchFlagValue(args []string, index int, flag string) (string, int, error) {
	if index+1 >= len(args) {
		return "", index, fmt.Errorf("%s requires a value", flag)
	}
	if isLaunchFlag(args[index+1]) {
		return "", index, fmt.Errorf("%s requires a value", flag)
	}
	return args[index+1], index + 1, nil
}

func isLaunchFlag(arg string) bool {
	switch arg {
	case "--root", "--root-policy", "--portable", "--detached", "--headless":
		return true
	default:
		return false
	}
}

func parseRootPolicyValue(value string) (RootPolicy, error) {
	switch RootPolicy(value) {
	case RootPolicyPortable, RootPolicyDetached:
		return RootPolicy(value), nil
	default:
		return "", fmt.Errorf("unknown root policy %q", value)
	}
}

func mergeLaunchPolicy(opts *launchOptions, policy RootPolicy) error {
	if opts.RootPolicy == "" || opts.RootPolicy == policy {
		opts.RootPolicy = policy
		return nil
	}
	return fmt.Errorf("conflicting root policy overrides %q and %q", opts.RootPolicy, policy)
}
