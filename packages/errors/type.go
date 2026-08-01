package errors

// Type categorizes an Error for programmatic handling (e.g. deciding
// whether a caller should retry, surface a 404, or treat the failure as
// fatal). It is a small, closed taxonomy — callers needing finer-grained
// identification should use Error.Code instead of adding new Types.
type Type string

const (
	TypeUnknown          Type = "UNKNOWN"
	TypeInvalidInput     Type = "INVALID_INPUT"
	TypeNotFound         Type = "NOT_FOUND"
	TypeAlreadyExists    Type = "ALREADY_EXISTS"
	TypePermissionDenied Type = "PERMISSION_DENIED"
	TypeUnauthenticated  Type = "UNAUTHENTICATED"
	TypeUnavailable      Type = "UNAVAILABLE"
	TypeTimeout          Type = "TIMEOUT"
	TypeCanceled         Type = "CANCELED"
	TypeInternal         Type = "INTERNAL"
)
