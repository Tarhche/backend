package configs

import "time"

const (
	// defaultWorkloadVMHostSocket is where a vmhost serves its engine and where
	// its orchestrator reaches it: a directory the two share as a volume.
	defaultWorkloadVMHostSocket = "/run/vmhost/vmhost.sock"

	defaultWorkloadVMHostPortRange = "20000-29999"
	defaultWorkloadVMHostDisk      = 200 << 30 // 200 GiB

	// defaultWorkloadVMDockerImage is what a Docker VM boots from. The control
	// plane names it in the specs it sends and a vmhost is given it too, so the
	// two say the same thing unless somebody makes them differ.
	defaultWorkloadVMDockerImage = "docker:29-dind"

	defaultWorkloadVMDefaultImage = "ubuntu:24.04"

	defaultWorkloadVMMinMemory       = 128 << 20 // 128 MiB
	defaultWorkloadVMMinDisk         = 1 << 30   // 1 GiB
	defaultWorkloadVMDockerMinMemory = 512 << 20 // 512 MiB
	defaultWorkloadVMDockerMinDisk   = 4 << 30   // 4 GiB

	defaultWorkloadVMMaxCPUs   = 4
	defaultWorkloadVMMaxMemory = 8 << 30  // 8 GiB
	defaultWorkloadVMMaxDisk   = 50 << 30 // 50 GiB

	defaultWorkloadVMUserMaxVMs = 5
	defaultWorkloadVMUserCPUs   = 8
	defaultWorkloadVMUserMemory = 16 << 30  // 16 GiB
	defaultWorkloadVMUserDisk   = 200 << 30 // 200 GiB

	defaultWorkloadVMMaxLifetime   = 720 * time.Hour
	defaultWorkloadVMCPUOvercommit = 4

	defaultWorkloadVMDockerDefaultCPUs           = 2
	defaultWorkloadVMDockerDefaultMemory         = 2 << 30  // 2 GiB
	defaultWorkloadVMDockerDefaultDisk           = 20 << 30 // 20 GiB
	defaultWorkloadVMDockerDefaultPorts          = "80,443,8080"
	defaultWorkloadVMDockerDefaultIngress        = "allow"
	defaultWorkloadVMDockerDefaultEgress         = "allow"
	defaultWorkloadVMDockerDefaultPersistentDisk = true

	defaultWorkloadSnapshotUserMax = 10
	defaultWorkloadSnapshotBucket  = "workload-snapshots"

	defaultWorkloadNodeRequestTimeout     = 30 * time.Second
	defaultWorkloadNodeRequestConcurrency = 16
	defaultWorkloadDockerPullTimeout      = 10 * time.Minute
	defaultWorkloadDockerReadyTimeout     = 3 * time.Minute
)

// WorkloadVMHost holds the configuration of the vmhost: the service that runs
// one node's VMs on its engine, in the microsandbox container, and serves that
// engine to its orchestrator.
//
// What it offers to VMs is a budget rather than a measure of the host, so that
// a node is never asked for more than it was given: CPUs are whole vCPUs, and
// memory and disk are bytes.
type WorkloadVMHost struct {
	Socket string `usage:"Unix socket the engine is served on, which this vmhost's orchestrator shares." env:"WORKLOAD_VMHOST_SOCKET" long:"socket"`

	PortRange     string `usage:"Host ports a VM's published ports are given from, as first-last. What is given is kept across restarts, so a VM keeps its ports." env:"WORKLOAD_VMHOST_PORT_RANGE" long:"port-range"`
	AdvertiseHost string `usage:"Host this vmhost is reached at on its pair network, which is where its orchestrator dials a VM's published ports." env:"WORKLOAD_VMHOST_ADVERTISE_HOST" long:"advertise-host"`

	DockerImage string `usage:"Image a Docker VM boots from: a docker-in-docker image whose dockerd comes up with the VM." env:"WORKLOAD_VMHOST_DOCKER_IMAGE" long:"docker-image"`

	CPUs   uint   `usage:"vCPUs this node offers to VMs. Zero offers every CPU the host has." env:"WORKLOAD_VMHOST_CPUS" long:"cpus"`
	Memory uint64 `usage:"Memory, in bytes, this node offers to VMs. Zero offers 80% of this container's memory limit, less 512 MiB." env:"WORKLOAD_VMHOST_MEMORY" long:"memory"`
	Disk   uint64 `usage:"Disk, in bytes, this node offers to VMs." env:"WORKLOAD_VMHOST_DISK" long:"disk"`
}

// NewWorkloadVMHost returns the configuration of the vmhost, holding the
// defaults it runs with until the console overrides them.
func NewWorkloadVMHost() *WorkloadVMHost {
	return &WorkloadVMHost{
		Socket:      defaultWorkloadVMHostSocket,
		PortRange:   defaultWorkloadVMHostPortRange,
		DockerImage: defaultWorkloadVMDockerImage,
		Disk:        defaultWorkloadVMHostDisk,
	}
}

// WorkloadSnapshotStorage is the S3 bucket VM snapshots are kept in. It is not
// the bucket uploads are kept in: the nodes write archives to it and read them
// back, and the control plane deletes them.
type WorkloadSnapshotStorage struct {
	S3Endpoint  string `usage:"S3 compatible endpoint snapshots are kept at, as host:port." env:"WORKLOAD_SNAPSHOT_S3_ENDPOINT" long:"snapshot-s3-endpoint"`
	S3AccessKey string `usage:"S3 access key for the snapshots bucket." env:"WORKLOAD_SNAPSHOT_S3_ACCESS_KEY" long:"snapshot-s3-access-key"`
	S3SecretKey string `usage:"S3 secret key for the snapshots bucket." env:"WORKLOAD_SNAPSHOT_S3_SECRET_KEY" long:"snapshot-s3-secret-key"`
	S3Bucket    string `usage:"S3 bucket snapshots are kept in. It is made on first use when it is not there." env:"WORKLOAD_SNAPSHOT_S3_BUCKET" long:"snapshot-s3-bucket"`
	S3UseSSL    bool   `usage:"Whether the snapshots' S3 endpoint is reached over TLS." env:"WORKLOAD_SNAPSHOT_S3_USE_SSL" long:"snapshot-s3-use-ssl"`
}

func newWorkloadSnapshotStorage() WorkloadSnapshotStorage {
	return WorkloadSnapshotStorage{
		S3Bucket: defaultWorkloadSnapshotBucket,
	}
}
