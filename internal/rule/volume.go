package rule

import "github.com/Ju571nK/Chatter/pkg/models"

// HasVolume reports whether the series has usable real or tick volume.
// The zero value preserves historical stock/crypto behaviour.
func HasVolume(ctx models.AnalysisContext) bool {
	return ctx.VolumeQuality != models.VolumeNone
}
