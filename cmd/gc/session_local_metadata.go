package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/gastownhall/gascity/internal/beads"
)

var localLifecycleMetadataKeys = []string{
	"synced_at",
	"last_woke_at",
	"pending_create_claim",
}

func isLocalLifecycleMetadataKey(key string) bool {
	switch key {
	case "synced_at", "last_woke_at", "pending_create_claim":
		return true
	default:
		return false
	}
}

func setLocalOrDurable(store beads.Store, id, key, value string, stderr io.Writer) error {
	_, err := setLocalOrDurableResult(store, id, key, value, stderr)
	return err
}

func setLocalOrDurableResult(store beads.Store, id, key, value string, stderr io.Writer) (bool, error) {
	if store == nil {
		return false, fmt.Errorf("setting %s on %s: store is nil", key, id)
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if !isLocalLifecycleMetadataKey(key) {
		return false, setDurableSessionMetadata(store, id, key, value, stderr)
	}
	err := store.SetLocalString(id, key, value)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, beads.ErrLocalMetadataNotSupported) {
		fmt.Fprintf(stderr, "session beads: setting local %s on %s: %v\n", key, id, err) //nolint:errcheck
		return false, err
	}
	return false, setDurableSessionMetadata(store, id, key, value, stderr)
}

func setLocalOrDurableBatch(store beads.Store, id string, batch map[string]string, stderr io.Writer) error {
	for _, key := range localLifecycleMetadataKeys {
		value, ok := batch[key]
		if !ok {
			continue
		}
		if err := setLocalOrDurable(store, id, key, value, stderr); err != nil {
			return err
		}
	}
	return nil
}

func moveCreatedLocalLifecycleMetadata(store beads.Store, id string, batch map[string]string, stderr io.Writer) error {
	for _, key := range localLifecycleMetadataKeys {
		value, ok := batch[key]
		if !ok {
			continue
		}
		usedLocal, err := setLocalOrDurableResult(store, id, key, value, stderr)
		if err != nil {
			return err
		}
		if !usedLocal {
			continue
		}
		if err := setDurableSessionMetadata(store, id, key, "", stderr); err != nil {
			return err
		}
	}
	return nil
}

func getLocalOrDurable(store beads.Store, bead beads.Bead, key string) (string, bool, error) {
	if store != nil && isLocalLifecycleMetadataKey(key) && bead.ID != "" {
		value, ok, err := store.GetLocalString(bead.ID, key)
		switch {
		case err == nil && ok:
			return value, true, nil
		case err == nil:
			// Cache miss falls through to the durable snapshot below.
		case errors.Is(err, beads.ErrLocalMetadataNotSupported):
			// Unsupported stores keep using durable metadata.
		default:
			return "", false, fmt.Errorf("getting local %s on %s: %w", key, bead.ID, err)
		}
	}
	if bead.Metadata == nil {
		return "", false, nil
	}
	value, ok := bead.Metadata[key]
	return value, ok, nil
}

func hydrateLocalLifecycleMetadata(store beads.Store, bead *beads.Bead) error {
	if bead == nil {
		return nil
	}
	for _, key := range localLifecycleMetadataKeys {
		value, ok, err := getLocalOrDurable(store, *bead, key)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if bead.Metadata == nil {
			bead.Metadata = make(map[string]string, len(localLifecycleMetadataKeys))
		}
		bead.Metadata[key] = value
	}
	return nil
}

func setDurableSessionMetadata(store beads.Store, id, key, value string, stderr io.Writer) error {
	if stderr == nil {
		stderr = io.Discard
	}
	if err := store.SetMetadata(id, key, value); err != nil {
		fmt.Fprintf(stderr, "session beads: setting %s on %s: %v\n", key, id, err) //nolint:errcheck
		return err
	}
	return nil
}
