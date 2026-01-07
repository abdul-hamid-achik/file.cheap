variable "cluster_name" {
  description = "Name of the cluster"
  type        = string
}

variable "network_cidr" {
  description = "CIDR for the private network"
  type        = string
  default     = "10.0.0.0/16"
}

variable "subnet_cidr" {
  description = "CIDR for the subnet"
  type        = string
  default     = "10.0.1.0/24"
}

variable "network_zone" {
  description = "Network zone"
  type        = string
  default     = "eu-central"
}

variable "ssh_allowed_ips" {
  description = "IPs allowed to SSH"
  type        = list(string)
  default     = ["0.0.0.0/0", "::/0"]
}
