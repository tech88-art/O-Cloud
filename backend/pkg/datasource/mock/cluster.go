package mock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

// ErrClusterNotFound is returned by GetCluster when no cluster matches the id.
// Wrapped so callers can errors.Is against it (handler maps it to 404).
var ErrClusterNotFound = errors.New("cluster not found")

// clustersFile is the JSON fixture name under fixturesPath. The set-a-small
// dataset ships a composite document (clusters / nodes / npus / ...) — T101
// reads only the `clusters` array from it.
const clustersFile = "clusters.json"

// clusterFixture is the on-disk shape: a composite JSON document where T101
// only cares about the top-level `clusters` array. Other keys are ignored
// here and will be picked up by later mock files (T103/T104/...).
type clusterFixture struct {
	Clusters []*model.Cluster `json:"clusters"`
}

// loadClusters reads clustersFile from fixturesPath at most once per Source.
// Errors are cached too — once loading fails, subsequent calls return the
// same error rather than re-reading a broken file every request.
//
// When fixturesPath is empty the loader treats it as "no fixtures" and
// returns an empty slice (lets unit tests construct a Source with no disk
// dependency).
func (s *Source) loadClusters() ([]*model.Cluster, error) {
	s.clustersOnce.Do(func() {
		if s.fixturesPath == "" {
			s.clusters = nil
			return
		}
		path := filepath.Join(s.fixturesPath, clustersFile)
		raw, err := os.ReadFile(path) //nolint:gosec // path comes from server config, not user input
		if err != nil {
			s.clustersErr = fmt.Errorf("read clusters fixture %q: %w", path, err)
			return
		}
		var doc clusterFixture
		if err := json.Unmarshal(raw, &doc); err != nil {
			s.clustersErr = fmt.Errorf("parse clusters fixture %q: %w", path, err)
			return
		}
		s.clusters = doc.Clusters
	})
	return s.clusters, s.clustersErr
}

// ListClusters returns every cluster in the fixture set.
//
// PHASE-1: backed by configs/mock-data/set-a-small/clusters.json.
// PHASE-2: k8s.Source will list via clientset; this mock stays for tests.
func (s *Source) ListClusters(ctx context.Context) ([]*model.Cluster, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	clusters, err := s.loadClusters()
	if err != nil {
		return nil, err
	}
	// Return a shallow copy so callers can't mutate cached state. The
	// elements stay shared by pointer (Cluster is read-only after load).
	out := make([]*model.Cluster, len(clusters))
	copy(out, clusters)
	return out, nil
}

// GetCluster returns the cluster matching id or ErrClusterNotFound if none.
func (s *Source) GetCluster(ctx context.Context, id string) (*model.Cluster, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	clusters, err := s.loadClusters()
	if err != nil {
		return nil, err
	}
	for _, c := range clusters {
		if c != nil && c.ID == id {
			return c, nil
		}
	}
	return nil, ErrClusterNotFound
}

