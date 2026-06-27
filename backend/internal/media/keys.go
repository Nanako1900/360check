package media

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
)

// buildOriginalKey builds the deterministic COS object key for an original
// upload. The key lives under a per-upload prefix derived from the client_uuid so
// the STS policy can scope writes to exactly this upload and a replayed
// credentials request resolves to the same key. Layout:
//
//	media/<owner_type>/<owner_id>/<client_uuid>/original.jpg
//
// The directory prefix (everything up to and including the client_uuid) is what
// prefixOf returns and what the STS resource ARN restricts to.
func buildOriginalKey(ownerType oapi.MediaOwnerType, ownerID int64, clientUUID uuid.UUID) string {
	return fmt.Sprintf("media/%s/%d/%s/original.jpg", ownerType, ownerID, clientUUID.String())
}

// deriveKey builds the sibling key for a derived tier under the SAME per-upload
// prefix as the original, e.g. .../<client_uuid>/web.jpg and thumb.jpg.
func deriveKey(originalKey string, tier oapi.MediaTier) string {
	return prefixOf(originalKey) + string(tier) + ".jpg"
}

// prefixOf returns the directory prefix of a key (everything up to and including
// the final slash). The STS policy scopes to prefix*.
func prefixOf(key string) string {
	if i := strings.LastIndex(key, "/"); i >= 0 {
		return key[:i+1]
	}
	return ""
}

// etagMatches compares a COS-reported etag with a client-reported one, tolerating
// the surrounding quotes COS sometimes includes and case differences in the hex.
func etagMatches(cosEtag, clientEtag string) bool {
	norm := func(s string) string { return strings.ToLower(strings.Trim(s, `"`)) }
	return norm(cosEtag) == norm(clientEtag)
}
