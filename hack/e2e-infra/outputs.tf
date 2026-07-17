output "public_ip" {
  description = "Floating IP of the k3s node (SSH + Kubernetes API)"
  value       = opentelekomcloud_networking_floatingip_v2.node.address
}

output "private_ip" {
  description = "Private IP of the node inside the VPC"
  value       = opentelekomcloud_compute_instance_v2.node.access_ip_v4
}

output "subnet_id" {
  description = "Neutron subnet ID for the cloud provider [LoadBalancer] section"
  value       = local.subnet_id
}

output "vpc_id" {
  value = local.vpc_id
}

output "availability_zone" {
  value = var.availability_zone
}

output "ssh" {
  description = "SSH access to the node"
  value       = "ssh ubuntu@${opentelekomcloud_networking_floatingip_v2.node.address}"
}
