"use client";
import Link from "next/link";
import {usePathname} from "next/navigation";
import {type ReactNode} from "react";
import {
  AppShell,
  Burger,
  Group,
  Text,
  ActionIcon,
  Indicator,
  useMantineColorScheme,
  UnstyledButton,
} from "@mantine/core";
import {
  IconNotes,
  IconHome,
  IconFile,
  IconMessage,
  IconUser,
  IconSettings,
  IconMoon,
  IconSun,
  IconBell,
  IconLogout,
} from "@tabler/icons-react";
import {useDisclosure} from "@mantine/hooks";
import {logout} from "@/features/dashboard/actions/logout";
import classes from "./layout.module.css";

type Props = {
  children: ReactNode;
};

const mockdata = [
  {label: "داشبرد", icon: IconHome, href: "/dashboard"},
  {
    label: "مقالات",
    icon: IconNotes,
    href: "/dashboard/articles",
  },
  {
    label: "کامنت ها",
    icon: IconMessage,
    href: "/dashboard/comments",
  },
  {label: "فایل ها", icon: IconFile, href: "/dashboard/files"},
  {label: "حساب", icon: IconUser, href: "/dashboard/account"},
  {label: "تنظیمات", icon: IconSettings, href: "/dashboard/setting"},
];

export function DashboardLayout({children}: Props) {
  const pathname = usePathname();
  const [mobileOpened, {toggle: toggleMobile}] = useDisclosure();
  const [desktopOpened, {toggle: toggleDesktop}] = useDisclosure(true);
  const {toggleColorScheme} = useMantineColorScheme();

  const links = mockdata.map((item) => (
    <UnstyledButton
      component={Link}
      className={classes.link}
      href={item.href}
      key={item.label}
      mb={5}
      data-active={pathname === item.href || undefined}
    >
      <item.icon className={classes.linkIcon} stroke={1.5} />
      <span>{item.label}</span>
    </UnstyledButton>
  ));

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
          <Burger
            opened={mobileOpened}
            onClick={toggleMobile}
            hiddenFrom="sm"
            size="sm"
          />
          <Group>
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
          <Group h="100%" px="md" align="center">
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
      <AppShell.Navbar>
        <nav className={classes.navbar}>
          <div className={classes.navbarMain}>{links}</div>
          <div className={classes.footer}>
            <form action={logout}>
              <UnstyledButton w={"100%"} type="submit" className={classes.link}>
                <IconLogout className={classes.linkIcon} stroke={1.5} />
                <span>خروج</span>
              </UnstyledButton>
            </form>
          </div>
        </nav>
      </AppShell.Navbar>
      <AppShell.Main>{children}</AppShell.Main>
    </AppShell>
  );
}
