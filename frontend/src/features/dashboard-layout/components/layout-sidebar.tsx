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
import {hasPermission} from "@/lib/auth/shared";
import {APP_PATHS} from "@/lib/app-paths";
import {Permissions} from "@/lib/app-permissions";
import classes from "./layout.module.css";

type Props = {
  userPermissions: string[];
};

const dashboard = APP_PATHS.dashboard;

type SidebarSchema = {
  label: string;
  icon: any;
  href: string;
  requiredPermissions: Permissions[];
};

const SIDE_BAR_DATA: SidebarSchema[] = [
  {
    label: "داشبرد",
    icon: IconHome,
    href: dashboard.index,
    requiredPermissions: [],
  },
  {
    label: "مقالات",
    icon: IconNotes,
    href: dashboard.articles.index,
    requiredPermissions: ["articles.index"],
  },
  {
    label: "کامنت ها",
    icon: IconMessages,
    href: dashboard.comments.index,
    requiredPermissions: ["comments.index"],
  },
  {
    label: "فایل ها",
    icon: IconFile,
    href: dashboard.files,
    requiredPermissions: ["files.index"],
  },
  {
    label: "کامنت های من",
    icon: IconMessage,
    href: dashboard.my.comments,
    requiredPermissions: ["self.comments.index"],
  },
  {
    label: "بوکمارک های من",
    icon: IconBookmarks,
    href: dashboard.my.bookmarks,
    requiredPermissions: ["self.bookmarks.index"],
  },
  {
    label: "کاربرها",
    icon: IconUsers,
    href: dashboard.users.index,
    requiredPermissions: ["users.index"],
  },
  {
    label: "نقش ها",
    icon: IconKey,
    href: dashboard.roles.index,
    requiredPermissions: ["roles.index"],
  },
  {
    label: "تنظیمات",
    icon: IconSettings,
    href: dashboard.settings,
    requiredPermissions: ["config.show"],
  },
  {
    label: "پروفایل",
    icon: IconUser,
    href: dashboard.profile.index,
    requiredPermissions: [],
  },
];

export function LayoutSidebar({userPermissions}: Props) {
  const pathname = usePathname();

  return SIDE_BAR_DATA.map((item) => {
    const hasAccess = hasPermission(userPermissions, item.requiredPermissions);

    if (hasAccess) {
      return (
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
      );
    }

    return null;
  });
}
