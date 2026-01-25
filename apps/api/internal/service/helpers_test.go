// Test helpers for pointer values in service tests.
package service_test

// ptrBool returns a pointer to v.
func ptrBool(v bool) *bool {
	return &v
}

// ptrInt returns a pointer to v.
func ptrInt(v int) *int {
	return &v
}

// ptrString returns a pointer to v.
func ptrString(v string) *string {
	return &v
}
