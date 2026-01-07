terraform {
  required_providers {
    hcloud = {
      source  = "hetznercloud/hcloud"
      version = "~> 1.45"
    }
  }
}

resource "hcloud_volume" "postgres" {
  name     = "${var.cluster_name}-postgres"
  size     = var.postgres_size
  location = var.location
  format   = "ext4"

  labels = {
    cluster = var.cluster_name
    service = "postgres"
  }
}

resource "hcloud_volume" "redis" {
  name     = "${var.cluster_name}-redis"
  size     = var.redis_size
  location = var.location
  format   = "ext4"

  labels = {
    cluster = var.cluster_name
    service = "redis"
  }
}

resource "hcloud_volume" "minio" {
  name     = "${var.cluster_name}-minio"
  size     = var.minio_size
  location = var.location
  format   = "ext4"

  labels = {
    cluster = var.cluster_name
    service = "minio"
  }
}

resource "hcloud_volume_attachment" "postgres" {
  volume_id = hcloud_volume.postgres.id
  server_id = var.storage_server_id
  automount = false
}

resource "hcloud_volume_attachment" "redis" {
  volume_id = hcloud_volume.redis.id
  server_id = var.storage_server_id
  automount = false
}

resource "hcloud_volume_attachment" "minio" {
  volume_id = hcloud_volume.minio.id
  server_id = var.storage_server_id
  automount = false
}
