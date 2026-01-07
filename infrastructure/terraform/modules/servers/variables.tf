variable "cluster_name" {
  description = "Name of the cluster"
  type        = string
}

variable "ssh_public_key" {
  description = "SSH public key for server access"
  type        = string
}

variable "master_server_type" {
  description = "Server type for master node"
  type        = string
  default     = "cx21"
}

variable "worker_server_type" {
  description = "Server type for worker nodes"
  type        = string
  default     = "cx31"
}

variable "worker_count" {
  description = "Number of worker nodes"
  type        = number
  default     = 2
}

variable "image" {
  description = "OS image"
  type        = string
  default     = "ubuntu-24.04"
}

variable "location" {
  description = "Hetzner datacenter location"
  type        = string
  default     = "fsn1"
}

variable "network_id" {
  description = "ID of the private network"
  type        = string
}

variable "firewall_ids" {
  description = "List of firewall IDs to attach"
  type        = list(string)
  default     = []
}

variable "master_private_ip" {
  description = "Private IP for master node"
  type        = string
  default     = "10.0.1.10"
}

variable "worker_ip_range" {
  description = "IP range for workers (will use .1, .2, etc)"
  type        = string
  default     = "10.0.1.20/24"
}
