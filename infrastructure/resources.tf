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
  id = "/dev/xvdf:vol-0d2bab5e75ac580e9:${aws_instance.backend.id}"
}

resource "aws_volume_attachment" "backend" {
  device_name = "/dev/xvdf"
  instance_id = aws_instance.backend.id
  volume_id   = aws_ebs_volume.backend.id
}

import {
  to = aws_instance.backend
  id = "i-026c60a5a3cdec06e"
}

resource "aws_instance" "backend" {
  ami               = "ami-0a628e1e89aaedf80" # Canonical, Ubuntu, 24.04, amd64 noble image
  instance_type     = "t2.micro"
  key_name          = "backend"
  availability_zone = "eu-central-1b"

  user_data = <<-EOT
    #!/bin/bash

    # volumes
    sudo mkfs.ext4 /dev/xvdf
    sudo mkdir /volume_01
    sudo mount /dev/xvdf /volume_01
    sudo echo "/dev/xvdf /volume_01 ext4 defaults,nofail 0 0" | sudo tee -a /etc/fstab

    # tools
    sudo apt install -y wget python3 ca-certificates curl htop jq vim

    # Add Docker's official GPG key:
    sudo install -m 0755 -d /etc/apt/keyrings
    sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
    sudo chmod a+r /etc/apt/keyrings/docker.asc

    # Add the repository to Apt sources:
    echo \
      "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu \
      $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | \
      sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
    sudo apt-get update

    # install docker and sysbox
    sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
    wget https://downloads.nestybox.com/sysbox/releases/v0.6.5/sysbox-ce_0.6.5-0.linux_amd64.deb
    sudo apt install -y ./sysbox-ce_0.6.5-0.linux_amd64.deb
    rm ./sysbox-ce_0.6.5-0.linux_amd64.deb

    # setup
    sudo systemctl enable docker.service
    sudo systemctl start docker.service
    sudo usermod -a -G docker ubuntu
    id ubuntu
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

import {
  to = aws_route53_zone.tarhche_com
  id = "Z0951095A7CDVGITDCUP"
}

resource "aws_route53_zone" "tarhche_com" {
  name          = "tarhche.com"
  force_destroy = false
  comment       = ""

  tags = {
    project_name = var.project_name
  }
}

import {
  to = aws_route53_record.a_record_tarhche_com
  id = "${aws_route53_zone.tarhche_com.id}_tarhche.com_A"
}

resource "aws_route53_record" "a_record_tarhche_com" {
  zone_id = aws_route53_zone.tarhche_com.id
  name    = "tarhche.com"
  type    = "A"
  ttl     = 300
  records = ["3.125.118.7"]
}

import {
  to = aws_route53_zone.tarhche_ir
  id = "Z07817351L3HY3TPTD5IU"
}

resource "aws_route53_zone" "tarhche_ir" {
  name          = "tarhche.ir"
  force_destroy = false
  comment       = ""

  tags = {
    project_name = var.project_name
  }
}

import {
  to = aws_route53_record.a_record_tarhche_ir
  id = "${aws_route53_zone.tarhche_ir.id}_tarhche.ir_A"
}

resource "aws_route53_record" "a_record_tarhche_ir" {
  zone_id = aws_route53_zone.tarhche_ir.id
  name    = "tarhche.ir"
  type    = "A"
  ttl     = 300
  records = ["3.125.118.7"]
}

import {
  to = aws_s3_bucket.tarhche-backend
  id = "tarhche-backend"
}

resource "aws_s3_bucket" "tarhche-backend" {
  bucket        = "tarhche-backend"
  force_destroy = false

  tags = {
    project_name = var.project_name
  }
}
