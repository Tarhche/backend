"use client";
import Link from "next/link";
import {
  AppShell,
  Burger,
  Group,
  Text,
  ActionIcon,
  Indicator,
  useMantineColorScheme,
} from "@mantine/core";
import {IconMoon, IconSun, IconBell} from "@tabler/icons-react";
import {useDisclosure} from "@mantine/hooks";
import classes from "./layout.module.css";

type Props = {
  children: React.ReactNode;
};

export function LayoutShell({children}: Props) {
  const [mobileOpened, {toggle: toggleMobile}] = useDisclosure();
  const [desktopOpened, {toggle: toggleDesktop}] = useDisclosure(true);
  const {toggleColorScheme} = useMantineColorScheme();

  return (
    <AppShell
      header={{height: 60}}
      navbar={{
        width: 300,
        breakpoint: "sm",
        collapsed: {mobile: !mobileOpened, desktop: !desktopOpened},
      }}
      padding="md"
    >
      <AppShell.Header>
        <Group h="100%" px={16} justify="space-between" align="center">
          <Group>
            <Burger
              opened={mobileOpened}
              onClick={toggleMobile}
              hiddenFrom="sm"
              size="sm"
            />
            <Burger
              opened={desktopOpened}
              onClick={toggleDesktop}
              visibleFrom="sm"
              size="sm"
            />
            <Text component={Link} href={"/"}>
              طرحچه
            </Text>
          </Group>
          <Group h="100%" align="center">
            <Indicator color="red" size={8} offset={2}>
              <ActionIcon variant="light" size="lg" radius="md">
                <IconBell style={{width: "70%", height: "70%"}} stroke={1.5} />
              </ActionIcon>
            </Indicator>
            <Indicator disabled>
              <ActionIcon
                variant="light"
                size="lg"
                radius="md"
                onClick={toggleColorScheme}
              >
                <IconSun
                  style={{width: "70%", height: "70%"}}
                  stroke={1.5}
                  className={classes.light}
                />
                <IconMoon
                  style={{width: "70%", height: "70%"}}
                  stroke={1.5}
                  className={classes.dark}
                />
              </ActionIcon>
            </Indicator>
          </Group>
        </Group>
      </AppShell.Header>
      {children}
    </AppShell>
  );
}

export const LayoutNavbar = AppShell.Navbar;
export const LayoutMain = AppShell.Main;
