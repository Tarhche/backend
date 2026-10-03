package vm

import "context"

// ImagePruner is an ImageStore that lets go of the images nothing boots
// itself, least recently used first, once they take more disk than its cache
// may: it knows their sizes and when each was last used, and keeps what it was
// asked for moments ago, which may be about to be booted.
//
// Which images VMs still boot is vmhost's to know, not the store's, so vmhost
// names them (keep, by digest), and the store lets go of the rest as it sees
// fit. It is optional: vmhost lets go of the least recently used images itself,
// through List and Remove, when its store does not do it.
type ImagePruner interface {
	// Prune lets go of the least recently used images not in keep until
	// those left take no more disk than the cache may, and says which it let
	// go of.
	Prune(ctx context.Context, keep []string) ([]Image, error)
}
