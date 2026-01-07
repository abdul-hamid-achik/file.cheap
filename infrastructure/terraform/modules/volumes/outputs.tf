output "postgres_volume_id" {
  description = "ID of PostgreSQL volume"
  value       = hcloud_volume.postgres.id
}

output "postgres_volume_path" {
  description = "Linux device path for PostgreSQL volume"
  value       = hcloud_volume.postgres.linux_device
}

output "redis_volume_id" {
  description = "ID of Redis volume"
  value       = hcloud_volume.redis.id
}

output "redis_volume_path" {
  description = "Linux device path for Redis volume"
  value       = hcloud_volume.redis.linux_device
}

output "minio_volume_id" {
  description = "ID of MinIO volume"
  value       = hcloud_volume.minio.id
}

output "minio_volume_path" {
  description = "Linux device path for MinIO volume"
  value       = hcloud_volume.minio.linux_device
}
