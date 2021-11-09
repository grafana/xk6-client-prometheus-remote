package remotewrite

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEvaluateTemplate(t *testing.T) {
	require.Equal(t, evaluateTemplate("something ${series_id} else", 12), "something 12 else")
	require.Equal(t, evaluateTemplate("something ${series_id/6} else", 12), "something 2 else")
}
