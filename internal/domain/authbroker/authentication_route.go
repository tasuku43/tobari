package authbroker

// AuthenticationRoute names who owns the real credential for one runtime
// request binding. It is an unordered routing fact, not a security level.
type AuthenticationRoute string

const (
	AuthenticationRouteBrokerRequired              AuthenticationRoute = "broker_required"
	AuthenticationRouteWorkspaceOwnedCompatibility AuthenticationRoute = "workspace_owned_compatibility"
)
