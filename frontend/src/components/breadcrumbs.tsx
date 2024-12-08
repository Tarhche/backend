import Link from "next/link";
import {Breadcrumbs as MantineBreadcrumbs, Anchor, Text} from "@mantine/core";
import classes from "./breadcrumbs.module.css";

type Props = {
  crumbs: {
    label: string;
    href?: string;
  }[];
};

export function Breadcrumbs({crumbs}: Props) {
  return (
    <MantineBreadcrumbs
      separator="\"
      classNames={{
        separator: classes.separator,
      }}
    >
      {crumbs.map((crumb) => {
        if (crumb.href) {
          return (
            <Anchor
              c={"dimmed"}
              size="md"
              component={Link}
              key={crumb.label}
              href={crumb.href}
            >
              {crumb.label}
            </Anchor>
          );
        }
        return (
          <Text c="dimmed" size="md" key={crumb.label}>
            {crumb.label}
          </Text>
        );
      })}
    </MantineBreadcrumbs>
  );
}
