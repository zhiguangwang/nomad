package storage

import "context"

type CSIPlugin interface {
	// Fingerprint returns a stream of ....
	Fingerprint(ctx context.Context) (<-chan *FingerprintResponse, error)

	ControllerPublishVolume(ctx context.Context) (*VolumeAttachmentResponse, error)
	DevicePublishVolume(ctx context.Context) (*VolumeAttachmentResponse, error)
}

// FingerprintResponse includes a set of detected csi interfaces and capabilities
type FingerprintResponse struct {
	// Error is populated when fingerprinting has failed.
	Error error
}

type VolumeAttachmentResponse struct {
	AttachmentPath string
}
