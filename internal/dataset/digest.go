package dataset

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
)

func membershipDigest(items []domain.DatasetItem) string {
	ordered := append([]domain.DatasetItem(nil), items...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].CaptureID == ordered[j].CaptureID {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].CaptureID < ordered[j].CaptureID
	})
	h := sha256.New()
	for _, item := range ordered {
		fmt.Fprintf(h, "%s\x00%s\x00%d\n", item.ID, item.CaptureID, item.Revision)
	}
	return hex.EncodeToString(h.Sum(nil))
}
