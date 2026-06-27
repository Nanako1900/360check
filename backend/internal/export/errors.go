package export

import "errors"

// ErrNotFound is returned when an export_jobs row for the given job_uuid is absent.
var ErrNotFound = errors.New("export: job not found")

// ErrInvalidType is returned when the requested export type is not one of the
// three supported values.
var ErrInvalidType = errors.New("export: invalid export type")

// ErrInvalidStatus is returned when an INSPECTION_RECORDS export carries a
// status filter that is not a frozen inspection_status enum literal.
var ErrInvalidStatus = errors.New("export: invalid inspection status")
