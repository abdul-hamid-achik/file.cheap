variable "cluster_name" {
  description = "Name of the cluster"
  type        = string
}

variable "location" {
  description = "Hetzner datacenter location"
  type        = string
  default     = "fsn1"
}

variable "postgres_size" {
  description = "Size of PostgreSQL volume in GB"
  type        = number
  default     = 20
}

variable "redis_size" {
  description = "Size of Redis volume in GB"
  type        = number
  default     = 10
}

variable "minio_size" {
  description = "Size of MinIO volume in GB"
  type        = number
  default     = 50
}

variable "storage_server_id" {
  description = "ID of the server to attach volumes to"
  type        = string
}
