"use client";
import {type ReactNode} from "react";
import Link from "next/link";
import {AppShell, Burger, Group, UnstyledButton} from "@mantine/core";
import {useDisclosure} from "@mantine/hooks";
import {AuthButtons} from "./auth-button";

type Props = {
  children: ReactNode;
};

export function AppMainShell({children}: Props) {
  const [opened, {toggle}] = useDisclosure();

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
              <AuthButtons />
            </Group>
          </Group>
        </Group>
      </AppShell.Header>
      <AppShell.Navbar py="md" px={4}>
        <AuthButtons />
      </AppShell.Navbar>
      <AppShell.Main px={"xl"}>{children}</AppShell.Main>
    </AppShell>
  );
}
