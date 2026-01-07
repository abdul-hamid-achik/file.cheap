variable "hcloud_token" {
  description = "Hetzner Cloud API token"
  type        = string
  sensitive   = true
}

variable "cluster_name" {
  description = "Name of the cluster"
  type        = string
  default     = "file-cheap"
}

variable "domain" {
  description = "Domain name for the application"
  type        = string
  default     = "file.cheap"
}

variable "ssh_public_key" {
  description = "SSH public key for server access"
  type        = string
}

variable "ssh_allowed_ips" {
  description = "IPs allowed to SSH (CIDR format)"
  type        = list(string)
  default     = ["0.0.0.0/0", "::/0"]
}

variable "location" {
  description = "Hetzner datacenter location (fsn1, nbg1, hel1)"
  type        = string
  default     = "fsn1"
}

variable "master_server_type" {
  description = "Server type for K3s master"
  type        = string
  default     = "cx21"
}

variable "worker_server_type" {
  description = "Server type for K3s workers"
  type        = string
  default     = "cx31"
}

variable "worker_count" {
  description = "Number of K3s worker nodes"
  type        = number
  default     = 2
}

variable "postgres_volume_size" {
  description = "PostgreSQL volume size in GB"
  type        = number
  default     = 20
}

variable "redis_volume_size" {
  description = "Redis volume size in GB"
  type        = number
  default     = 10
}

variable "minio_volume_size" {
  description = "MinIO volume size in GB"
  type        = number
  default     = 50
}
