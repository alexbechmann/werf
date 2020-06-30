package common

import (
	"fmt"
	"strings"

	"github.com/werf/werf/pkg/storage/synchronization_server"

	"github.com/werf/werf/pkg/storage"
	"github.com/werf/werf/pkg/werf"
)

func GetStagesStorageCache(synchronization string) (storage.StagesStorageCache, error) {
	if synchronization == storage.LocalStorageAddress {
		return storage.NewFileStagesStorageCache(werf.GetStagesStorageCacheDir()), nil
	} else if strings.HasPrefix(synchronization, "kubernetes://") {
		ns := strings.TrimPrefix(synchronization, "kubernetes://")
		return storage.NewKubernetesStagesStorageCache(ns), nil
	} else if strings.HasPrefix(synchronization, "http://") || strings.HasPrefix(synchronization, "https://") {
		return synchronization_server.NewStagesStorageCacheHttpClient(fmt.Sprintf("%s/stages-storage-cache", synchronization)), nil
	} else {
		panic(fmt.Sprintf("unknown synchronization param %q", synchronization))
	}
}

func GetStorageLockManager(synchronization string) (storage.LockManager, error) {
	if synchronization == storage.LocalStorageAddress {
		return storage.NewGenericLockManager(werf.GetHostLocker()), nil
	} else if strings.HasPrefix(synchronization, "kubernetes://") {
		ns := strings.TrimPrefix(synchronization, "kubernetes://")
		return storage.NewKubernetesLockManager(ns), nil
	} else if strings.HasPrefix(synchronization, "http://") || strings.HasPrefix(synchronization, "https://") {
		return synchronization_server.NewLockManagerHttpClient(fmt.Sprintf("%s/lock-manager", synchronization)), nil
	} else {
		panic(fmt.Sprintf("unknown synchronization param %q", synchronization))
	}
}
