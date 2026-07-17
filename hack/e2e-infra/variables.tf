variable "prefix" {
  description = "Name prefix for all created resources"
  type        = string
  default     = "ccm-e2e"
}

variable "network_id" {
  description = "OTC network UUID of the subnet the node attaches to (openstack subnet list, column Network)"
  type        = string
}

variable "subnet_id" {
  description = "Neutron subnet UUID used by the cloud provider for load balancer VIPs (openstack subnet list, column ID)"
  type        = string
}

variable "vpc_id" {
  description = "VPC (router) ID, passed through to the cloud provider config"
  type        = string
  default     = ""
}

variable "availability_zone" {
  description = "AZ for the node and for load balancers"
  type        = string
  default     = "eu-de-01"
}

variable "flavor_name" {
  description = "ECS flavor for the k3s node"
  type        = string
  default     = "s3.large.2"
}

variable "image_name" {
  description = "Public image for the node"
  type        = string
  default     = "Standard_Ubuntu_24.04_amd64_bios_latest"
}

variable "k3s_channel" {
  description = "k3s release channel (stable/latest) or pinned version via INSTALL_K3S_VERSION semantics"
  type        = string
  default     = "stable"
}

variable "ssh_public_key_file" {
  description = "Path to the SSH public key for node access"
  type        = string
  default     = "~/.ssh/id_rsa.pub"
}

variable "admin_cidr" {
  description = "CIDR allowed to reach SSH (22) and the Kubernetes API (6443)"
  type        = string
  default     = "0.0.0.0/0"
}

variable "vpc_cidr" {
  description = "CIDR of the VPC, allowed full access to the node (load balancer health checks, NodePorts)"
  type        = string
  default     = "10.0.0.0/8"
}
