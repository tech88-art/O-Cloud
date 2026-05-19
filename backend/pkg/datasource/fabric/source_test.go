package fabric

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/ocloud-edge/backend/pkg/aggregator"
)

const validYAML = `
switches:
  - id: switch-tor-01
    name: switch-tor-01
    type: tor
    location: site-a-shanghai
    portsTotal: 48
    portsUsed: 3
    bandwidthGbps: 25
    status: up
  - id: switch-spine-01
    name: switch-spine-01
    type: spine
    portsTotal: 32
    status: up
links:
  - id: link-tor-01-worker-a-01
    from: worker-site-a-01
    to: switch-tor-01
    bandwidthGbps: 25
    medium: dac
    utilization: 12.0
  - id: link-spine-01-tor-01
    from: switch-tor-01
    to: switch-spine-01
    bandwidthGbps: 400
    medium: optical
`

func writeTempYAML(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fabric.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestNewSource_EmptyPath_ReturnsEmptySource(t *testing.T) {
	src, err := NewSource(Options{})
	require.NoError(t, err)
	switches, err := src.ListSwitches(context.Background())
	require.NoError(t, err)
	assert.Empty(t, switches)
	links, err := src.ListLinks(context.Background())
	require.NoError(t, err)
	assert.Empty(t, links)
}

func TestNewSource_HappyPath_ParsesSwitchesAndLinks(t *testing.T) {
	path := writeTempYAML(t, validYAML)
	src, err := NewSource(Options{Path: path})
	require.NoError(t, err)

	switches, err := src.ListSwitches(context.Background())
	require.NoError(t, err)
	require.Len(t, switches, 2)
	assert.Equal(t, "switch-tor-01", switches[0].ID)
	assert.Equal(t, "tor", switches[0].Type)
	assert.Equal(t, 48, switches[0].PortsTotal)
	assert.Equal(t, 25, switches[0].BandwidthGbps)
	assert.Equal(t, "up", switches[0].Status)
	assert.Equal(t, "spine", switches[1].Type)

	links, err := src.ListLinks(context.Background())
	require.NoError(t, err)
	require.Len(t, links, 2)
	assert.Equal(t, "worker-site-a-01", links[0].From)
	assert.Equal(t, "switch-tor-01", links[0].To)
	assert.Equal(t, 25, links[0].BandwidthGbps)
	assert.Equal(t, "dac", links[0].Medium)
	assert.InDelta(t, 12.0, links[0].Utilization, 0.001)
	assert.Equal(t, 400, links[1].BandwidthGbps)
	assert.Equal(t, "optical", links[1].Medium)
}

func TestNewSource_MissingFile_ReturnsErrParse(t *testing.T) {
	_, err := NewSource(Options{Path: "/nonexistent/fabric.yaml"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrParse)
}

func TestNewSource_MalformedYAML_ReturnsErrParse(t *testing.T) {
	path := writeTempYAML(t, "this is: not: valid: yaml: [")
	_, err := NewSource(Options{Path: path})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrParse)
}

func TestNewSource_EmptyFile_NoSwitchesNoLinks(t *testing.T) {
	path := writeTempYAML(t, "")
	src, err := NewSource(Options{Path: path})
	require.NoError(t, err, "empty YAML is valid (just no entries)")
	switches, _ := src.ListSwitches(context.Background())
	links, _ := src.ListLinks(context.Background())
	assert.Empty(t, switches)
	assert.Empty(t, links)
}

func TestListSwitches_ReturnsDefensiveCopy(t *testing.T) {
	path := writeTempYAML(t, validYAML)
	src, err := NewSource(Options{Path: path})
	require.NoError(t, err)

	out1, _ := src.ListSwitches(context.Background())
	require.Len(t, out1, 2)
	out1[0].Name = "MUTATED"

	out2, _ := src.ListSwitches(context.Background())
	require.Len(t, out2, 2)
	assert.Equal(t, "switch-tor-01", out2[0].Name,
		"mutation of one ListSwitches result must not leak to subsequent calls")
}

func TestListLinks_ReturnsDefensiveCopy(t *testing.T) {
	path := writeTempYAML(t, validYAML)
	src, err := NewSource(Options{Path: path})
	require.NoError(t, err)

	out1, _ := src.ListLinks(context.Background())
	require.Len(t, out1, 2)
	out1[0].Utilization = 99.0

	out2, _ := src.ListLinks(context.Background())
	require.Len(t, out2, 2)
	assert.InDelta(t, 12.0, out2[0].Utilization, 0.001)
}

func TestSource_NilSafetyOnListSwitches(t *testing.T) {
	var s *Source
	out, err := s.ListSwitches(context.Background())
	require.NoError(t, err)
	assert.Nil(t, out)
}

func TestSource_NilSafetyOnListLinks(t *testing.T) {
	var s *Source
	out, err := s.ListLinks(context.Background())
	require.NoError(t, err)
	assert.Nil(t, out)
}

func TestErrParse_IsExported(t *testing.T) {
	// Sanity: callers can errors.Is against the sentinel.
	wrapped := errors.New("fabric: parse failure: wrapped")
	assert.False(t, errors.Is(wrapped, ErrParse),
		"a non-wrapped errors.New shouldn't unify with the sentinel")
	wrapped2 := errors.Join(ErrParse, errors.New("detail"))
	assert.True(t, errors.Is(wrapped2, ErrParse))
}

// Sanity: ensure aggregator types round-trip through the YAML
// decoder without truncating fields that mock fixtures rely on.
func TestRoundTrip_PreservesVLANsAndRTT(t *testing.T) {
	yamlBody := `
switches:
  - id: sw
    name: sw
    type: tor
    status: up
    vlans:
      - vlan-mgmt
      - vlan-data
links:
  - id: l
    from: a
    to: b
    bandwidthGbps: 25
    rttUs: 0.85
`
	path := writeTempYAML(t, yamlBody)
	src, err := NewSource(Options{Path: path})
	require.NoError(t, err)
	switches, _ := src.ListSwitches(context.Background())
	require.Len(t, switches, 1)
	assert.Equal(t, []string{"vlan-mgmt", "vlan-data"}, switches[0].VLANs)
	links, _ := src.ListLinks(context.Background())
	require.Len(t, links, 1)
	assert.InDelta(t, 0.85, links[0].RTTUs, 0.001)
}

// We pulled in sigs.k8s.io/yaml because controller-runtime already
// depends on it transitively; verify we don't accidentally drop the
// import.
var _ = aggregator.NetworkSwitch{}
