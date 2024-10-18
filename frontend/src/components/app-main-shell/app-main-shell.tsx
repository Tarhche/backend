"use client";
import {ReactNode} from "react";
import Link from "next/link";
import {AppShell, Burger, Group, UnstyledButton} from "@mantine/core";
import {useDisclosure} from "@mantine/hooks";
import classes from "./app-main-shell.module.css";

type Links = {
  readonly label: string;
  readonly href: string;
};

const LINKS: Links[] = [
  {
    label: "ورود",
    href: "/auth/login",
  },
  {
    label: "عضویت",
    href: "/auth/register",
  },
];

type Props = {
  children: ReactNode;
};

export function AppMainShell({children}: Props) {
  const [opened, {toggle}] = useDisclosure();

  const AUTH_LINKS = LINKS.map((link) => {
    return (
      <UnstyledButton
        key={link.href}
        className={classes.control}
        component={Link}
        href={link.href}
      >
        {link.label}
      </UnstyledButton>
    );
  });

  return (
    <AppShell
      header={{height: 60}}
      navbar={{
        width: 300,
        breakpoint: "sm",
        collapsed: {desktop: true, mobile: !opened},
      }}
      padding="md"
    >
      <AppShell.Header px={"xl"}>
        <Group h="100%">
          <Burger opened={opened} onClick={toggle} hiddenFrom="sm" size="sm" />
          <Group justify="space-between" style={{flex: 1}}>
            <UnstyledButton fz={"h4"} component={Link} href={"/"}>
              طرح‌چه
            </UnstyledButton>
            <Group ml="xl" gap={0} visibleFrom="sm">
              {AUTH_LINKS}
            </Group>
          </Group>
        </Group>
      </AppShell.Header>
      <AppShell.Navbar py="md" px={4}>
        {AUTH_LINKS}
      </AppShell.Navbar>
      <AppShell.Main px={"xl"}>{children}</AppShell.Main>
    </AppShell>
  );
}
