package redisutil

import "fmt"

// Error used to represent an error
type Error string

func (e Error) Error() string { return string(e) }

// nodeNotFoundedError returns when a node is not present in the cluster
const nodeNotFoundedError = Error("node not founded")

// IsNodeNotFoundedError returns true if the current error is a NodeNotFoundedError
func IsNodeNotFoundedError(err error) bool {
	return err == nodeNotFoundedError
}

// ClusterInfosError error type for redis cluster infos access
type ClusterInfosError struct {
	errs         map[string]error
	partial      bool
	inconsistent bool
}

// NewClusterInfosError returns an instance of cluster infos error
func NewClusterInfosError() ClusterInfosError {
	return ClusterInfosError{
		errs:         make(map[string]error),
		partial:      false,
		inconsistent: false,
	}
}

// Error error string
func (e ClusterInfosError) Error() string {
	s := ""
	if e.partial {
		s += "Cluster infos partial: "
		for addr, err := range e.errs {
			s += fmt.Sprintf("%s: '%s'", addr, err)
		}
		return s
	}
	if e.inconsistent {
		s += "Cluster view is inconsistent"
	}
	return s
}

// Partial true if the some nodes of the cluster didn't answer
func (e ClusterInfosError) Partial() bool {
	return e.partial
}

// Inconsistent true if the nodes do not agree with each other
func (e ClusterInfosError) Inconsistent() bool {
	return e.inconsistent
}

// IsPartialError returns true if the error is due to partial data recovery
func IsPartialError(err error) bool {
	e, ok := err.(ClusterInfosError)
	return ok && e.Partial()
}

// IsInconsistentError eturns true if the error is due to cluster inconsistencies
func IsInconsistentError(err error) bool {
	e, ok := err.(ClusterInfosError)
	return ok && e.Inconsistent()
}
