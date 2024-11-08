"use client";
import Link from "next/link";
import {Breadcrumbs as MantineBreadcrumbs, Anchor} from "@mantine/core";

type Props = {
  crumbs: {
    href: string;
    label: string;
  }[];
};

export function Breadcrumbs({crumbs}: Props) {
  return (
    <MantineBreadcrumbs separator="\">
      {crumbs.map((item, index) => (
        <Anchor
          c={"dimmed"}
          size="lg"
          component={Link}
          key={index}
          href={item.href}
        >
          {item.label}
        </Anchor>
      ))}
    </MantineBreadcrumbs>
  );
}
