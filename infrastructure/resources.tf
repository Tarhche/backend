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
    sudo apt install -y wget python3 ca-certificates curl htop jq vim make

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
  id = "eipalloc-02bceef376bc05f89"
}

resource "aws_eip" "backend" {
  instance = aws_instance.backend.id
  domain   = "vpc"

  tags = {
    project_name = var.project_name
  }
}

import {
  to = aws_lb.tarhche
  id = "arn:aws:elasticloadbalancing:eu-central-1:381491955644:loadbalancer/app/tarhche/6953bf38e49158d7"
}

resource "aws_lb" "tarhche" {
  name                       = "tarhche"
  internal                   = false
  load_balancer_type         = "application"
  idle_timeout               = 60
  ip_address_type            = "ipv4"
  enable_deletion_protection = true

  security_groups = [
    aws_security_group.backend.id,
  ]

  subnets = [
    "subnet-0d68a01f5a4861c65",
    "subnet-0fca4d198b88d68d6",
    "subnet-0c8f8df628e715018",
  ]

  tags = {
    project_name = var.project_name
  }
}

import {
  to = aws_lb_target_group.http
  id = "arn:aws:elasticloadbalancing:eu-central-1:381491955644:targetgroup/HTTP/374d0a16b08c8d4a"
}

resource "aws_lb_target_group" "http" {
  name              = "HTTP"
  port              = 80
  protocol          = "HTTP"
  vpc_id            = "vpc-04db3e4490d90be8e"
  ip_address_type   = "ipv4"
  proxy_protocol_v2 = false

  lambda_multi_value_headers_enabled = false

  health_check {
    path                = "/"
    interval            = 30
    timeout             = 5
    healthy_threshold   = 5
    unhealthy_threshold = 2
  }

  tags = {
    project_name = var.project_name
  }
}

# resource "aws_lb_target_group_attachment" "backend_http" {
#   target_group_arn = aws_lb_target_group.http.arn
#   target_id        = aws_instance.backend.id
#   port             = 80
# }

import {
  to = aws_lb_listener.http
  id = "arn:aws:elasticloadbalancing:eu-central-1:381491955644:listener/app/tarhche/6953bf38e49158d7/637c8770b5e4d6ed"
}

resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.tarhche.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    order            = 1
    type             = "redirect"
    target_group_arn = aws_lb_target_group.http.arn

    redirect {
      host        = "#{host}"
      path        = "/#{path}"
      port        = "443"
      protocol    = "HTTPS"
      query       = "#{query}"
      status_code = "HTTP_301"
    }
  }

  tags = {
    project_name = var.project_name
  }
}

import {
  to = aws_lb_listener.https
  id = "arn:aws:elasticloadbalancing:eu-central-1:381491955644:listener/app/tarhche/6953bf38e49158d7/ab1c7847cbb6f739"
}

resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.tarhche.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = aws_acm_certificate.tarhche_com.arn

  default_action {
    order            = 1
    type             = "forward"
    target_group_arn = aws_lb_target_group.http.arn

    forward {
      stickiness {
        duration = 3600
        enabled  = false
      }

      target_group {
        arn    = aws_lb_target_group.http.arn
        weight = 1
      }
    }
  }

  tags = {
    project_name = var.project_name
  }
}

import {
  to = aws_route53domains_registered_domain.tarhche-com
  id = "tarhche.com"
}

resource "aws_route53domains_registered_domain" "tarhche-com" {
  domain_name = "tarhche.com"

  name_server {
    name = "ns-1611.awsdns-09.co.uk"
  }

  name_server {
    name = "ns-1254.awsdns-28.org"
  }

  name_server {
    name = "ns-143.awsdns-17.com"
  }

  name_server {
    name = "ns-769.awsdns-32.net"
  }

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

  alias {
    name                   = aws_lb.tarhche.dns_name
    zone_id                = aws_lb.tarhche.zone_id
    evaluate_target_health = true
  }
}

import {
  to = aws_route53_record.a_record_all_tarhche_com
  id = "${aws_route53_zone.tarhche_com.id}_*.tarhche.com_A"
}

resource "aws_route53_record" "a_record_all_tarhche_com" {
  zone_id = aws_route53_zone.tarhche_com.id
  name    = "*.tarhche.com"
  type    = "A"

  alias {
    name                   = aws_lb.tarhche.dns_name
    zone_id                = aws_lb.tarhche.zone_id
    evaluate_target_health = true
  }
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

  alias {
    evaluate_target_health = true
    name                   = aws_lb.tarhche.dns_name
    zone_id                = aws_lb.tarhche.zone_id
  }
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

import {
  to = aws_acm_certificate.tarhche_com
  id = "arn:aws:acm:eu-central-1:381491955644:certificate/a446a0ad-9cac-479f-a1d6-59b983d633d6"
}

resource "aws_acm_certificate" "tarhche_com" {
  domain_name       = "tarhche.com"
  validation_method = "DNS"

  subject_alternative_names = [
    "tarhche.com",
    "*.tarhche.com",
  ]

  lifecycle {
    create_before_destroy = true
  }

  tags = {
    project_name = var.project_name
  }
}

import {
  to = aws_route53_record.tarhche_com_ssl_validation
  id = "${aws_route53_zone.tarhche_com.id}__e7a6f01cbe22cb6d1db5c70fb80299a8.tarhche.com_CNAME"
}

resource "aws_route53_record" "tarhche_com_ssl_validation" {
  zone_id = aws_route53_zone.tarhche_com.id
  name    = "_e7a6f01cbe22cb6d1db5c70fb80299a8.tarhche.com"
  type    = "CNAME"
  records = ["_0fdeb4d57a8f62c9a90a8f77b0146a14.zfyfvmchrl.acm-validations.aws."]
  ttl     = 60
}
