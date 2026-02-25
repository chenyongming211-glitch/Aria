package datapath

import "errors"

var (
	// ErrInterfaceNotFound is returned when the WireGuard interface does not exist.
	ErrInterfaceNotFound = errors.New("interface not found")

	// ErrInterfaceExists is returned when trying to create an interface that already exists.
	ErrInterfaceExists = errors.New("interface already exists")

	// ErrPeerNotFound is returned when a peer with the given public key does not exist.
	ErrPeerNotFound = errors.New("peer not found")

	// ErrPeerExists is returned when trying to add a peer that already exists.
	ErrPeerExists = errors.New("peer already exists")

	// ErrInvalidKey is returned when a WireGuard key is invalid.
	ErrInvalidKey = errors.New("invalid wireguard key")

	// ErrInvalidEndpoint is returned when a peer endpoint is invalid.
	ErrInvalidEndpoint = errors.New("invalid endpoint")

	// ErrInvalidCIDR is returned when a CIDR notation is invalid.
	ErrInvalidCIDR = errors.New("invalid CIDR notation")

	// ErrRouteNotFound is returned when a route does not exist.
	ErrRouteNotFound = errors.New("route not found")

	// ErrRouteExists is returned when trying to add a route that already exists.
	ErrRouteExists = errors.New("route already exists")

	// ErrOperationNotSupported is returned when an operation is not supported.
	ErrOperationNotSupported = errors.New("operation not supported")

	// ErrNotImplemented is returned for features that are not yet implemented.
	ErrNotImplemented = errors.New("not implemented")

	// ErrPermissionDenied is returned when the operation requires elevated privileges.
	ErrPermissionDenied = errors.New("permission denied: operation requires root privileges")

	// ErrBGPNotRunning is returned when BGP operations are attempted but BGP is not running.
	ErrBGPNotRunning = errors.New("BGP daemon is not running")

	// ErrHAPeerNotConfigured is returned when HA operations are attempted without a peer.
	ErrHAPeerNotConfigured = errors.New("HA peer not configured")
)
