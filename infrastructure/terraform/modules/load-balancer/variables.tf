variable "cluster_name" {
  description = "Name of the cluster"
  type        = string
}

variable "location" {
  description = "Hetzner datacenter location"
  type        = string
  default     = "fsn1"
}

variable "load_balancer_type" {
  description = "Type of load balancer"
  type        = string
  default     = "lb11"
}

variable "network_id" {
  description = "ID of the private network"
  type        = string
}

variable "worker_ids" {
  description = "IDs of worker servers to target"
  type        = list(string)
}

variable "lb_private_ip" {
  description = "Private IP for the load balancer"
  type        = string
  default     = "10.0.1.5"
}
