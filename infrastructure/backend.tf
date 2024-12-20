provider "aws" {
  region = "eu-central-1"
}

variable "project_name" {
  description = "Project tag given to each deployed Instance"
  type        = string
}

variable "instance_name" {
  description = "instance_name"
  type        = string
}

import {
  to = aws_security_group.backend
  id = "sg-0c4446cdf14777251"
}

resource "aws_security_group" "backend" {
  name        = var.instance_name
  description = "Allow HTTP, HTTPS, and SSH inbound traffic"

  tags = {
    project_name = var.project_name
  }

  # Allow SSH (port 22) from any IP address
  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # Allow HTTP (port 80) from any IP address
  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"] # Allow HTTP from anywhere
  }

  # Allow HTTPS (port 443) from any IP address
  ingress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # Allow all outbound traffic
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1" # all protocols
    cidr_blocks = ["0.0.0.0/0"]
  }
}

import {
  to = aws_ebs_volume.backend
  id = "vol-0d2bab5e75ac580e9"
}

resource "aws_ebs_volume" "backend" {
  availability_zone = aws_instance.backend.availability_zone
  encrypted         = false
  size              = 10

  tags = {
    project_name = var.project_name
  }
}

import {
  to = aws_volume_attachment.backend
  id = "/dev/xvdf:vol-0d2bab5e75ac580e9:i-092b668f7d09a7e69"
}

resource "aws_volume_attachment" "backend" {
  device_name = "/dev/xvdf"
  instance_id = aws_instance.backend.id
  volume_id   = aws_ebs_volume.backend.id
}

import {
  to = aws_instance.backend
  id = "i-092b668f7d09a7e69"
}

resource "aws_instance" "backend" {
  ami               = "ami-0e54671bdf3c8ed8d" # Amazon linux 2023
  instance_type     = "t2.micro"
  key_name          = "backend"
  availability_zone = "eu-central-1b"

  user_data = <<-EOT
    #!/bin/bash
    sudo mkfs.ext4 /dev/xvdf
    sudo mkdir /volume_01
    sudo mount /dev/xvdf /volume_01
    sudo echo "/dev/xvdf /volume_01 ext4 defaults,nofail 0 0" | sudo tee -a /etc/fstab
    sudo yum install wget -y python3
    sudo yum install -y amazon-cloudwatch-agent jq htop vim docker
    sudo systemctl enable docker.service
    sudo systemctl start docker.service
    sudo usermod -a -G docker ec2-user
    id ec2-user
    newgrp docker

    docker swarm init --advertise-addr 192.168.99.100
  EOT

  root_block_device {
    delete_on_termination = true
    encrypted             = false
    volume_size           = 20
    volume_type           = "gp3"

    tags = {
      project_name = var.project_name
    }
  }

  security_groups = [
    aws_security_group.backend.name
  ]

  tags = {
    project_name = var.project_name
  }
}

import {
  to = aws_eip.backend
  id = "eipalloc-0adaac6f91269c716"
}

resource "aws_eip" "backend" {
  instance = aws_instance.backend.id
  domain   = "vpc"

  tags = {
    project_name = var.project_name
  }
}
