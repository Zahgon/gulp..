//go:build !darwin

package watch

// normalizeUnicode is a no-op everywhere except darwin.
//
// Linux and Windows return filenames byte-for-byte as they were created, so
// there is no composed/decomposed mismatch to reconcile and normalizing would
// only cost time.
func normalizeUnicode(path string) string { return path }
