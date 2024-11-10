"use client";
import Link from "next/link";
import {usePathname} from "next/navigation";
import {UnstyledButton} from "@mantine/core";
import {
  IconNotes,
  IconHome,
  IconFile,
  IconMessage,
  IconSettings,
  IconBookmarks,
  IconMessages,
  IconUsers,
  IconKey,
  IconUser,
} from "@tabler/icons-react";
import {APP_PATHS} from "@/lib/app-paths";
import classes from "./layout.module.css";

export function LayoutSidebar() {
  const pathname = usePathname();
  const dashboard = APP_PATHS.dashboard;
  const SIDE_BAR_DATA = [
    {label: "داشبرد", icon: IconHome, href: dashboard.index},
    {
      label: "مقالات",
      icon: IconNotes,
      href: dashboard.articles.index,
    },
    {
      label: "کامنت ها",
      icon: IconMessages,
      href: dashboard.comments.index,
    },
    {label: "فایل ها", icon: IconFile, href: dashboard.files},
    {label: "کامنت های من", icon: IconMessage, href: dashboard.comments.index},
    {
      label: "بوکمارک های من",
      icon: IconBookmarks,
      href: dashboard.my.bookmarks,
    },
    {label: "کاربرها", icon: IconUsers, href: dashboard.users.index},
    {label: "نقش ها", icon: IconKey, href: dashboard.roles.index},
    {label: "تنظیمات", icon: IconSettings, href: dashboard.settings},
    {label: "پروفایل", icon: IconUser, href: dashboard.profile},
  ];

  return SIDE_BAR_DATA.map((item) => (
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
}
