package media

import "errors"

// Sentinel errors mapped to envelope codes by the handler.
var (
	// ErrNotFound — media id/client_uuid missing or soft-deleted.
	ErrNotFound = errors.New("media asset not found")
	// ErrTierNotOriginal — upload-credentials requested for a non-original tier.
	// Per D4 the APP uploads ONLY tier=original; web/thumb are backend-derived.
	ErrTierNotOriginal = errors.New("only tier=original may be uploaded")
	// ErrVerifyFailed — confirm's HeadObject found the key missing or the
	// etag/size did not match the client's report. The row stays UPLOADING.
	ErrVerifyFailed = errors.New("media object verification failed")
	// ErrNotConfirmed — derive ran against an original that is not CONFIRMED.
	ErrNotConfirmed = errors.New("original media asset is not confirmed")
	// ErrNotOriginal — derive ran against a non-original tier row.
	ErrNotOriginal = errors.New("derive source is not the original tier")
	// ErrInvalidOwnerType — owner_type is not one of the frozen enum literals.
	ErrInvalidOwnerType = errors.New("invalid owner_type")
)
