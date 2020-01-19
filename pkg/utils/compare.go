package utils

import (
	"fmt"

	"github.com/go-logr/logr"
)

func CompareIntValue(name string, old, new *int32, reqLogger logr.Logger) bool {
	if old == nil && new == nil {
		return true
	} else if old == nil || new == nil {
		return false
	} else if *old != *new {
		reqLogger.V(4).Info(fmt.Sprintf("compare status.%s: %d - %d", name, *old, *new))
		return true
	}

	return false
}

func CompareInt32(name string, old, new int32, reqLogger logr.Logger) bool {
	if old != new {
		reqLogger.V(4).Info(fmt.Sprintf("compare status.%s: %d - %d", name, old, new))
		return true
	}

	return false
}

func CompareStringValue(name string, old, new string, reqLogger logr.Logger) bool {
	if old != new {
		reqLogger.V(4).Info(fmt.Sprintf("compare %s: %s - %s", name, old, new))
		return true
	}

	return false
}
