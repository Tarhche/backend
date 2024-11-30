import {UnstyledButton, ScrollArea} from "@mantine/core";
import {IconLogout} from "@tabler/icons-react";
import {LayoutShell, LayoutMain, LayoutNavbar} from "./layout-shell";
import {LayoutSidebar} from "./layout-sidebar";
import {logout} from "@/features/auth/actions";
import {getUserPermissions} from "@/lib/auth";
import classes from "./layout.module.css";

type Props = {
  children: React.ReactNode;
};

export function DashboardLayout({children}: Props) {
  const userPermissions = getUserPermissions();

  return (
    <LayoutShell>
      <LayoutNavbar className={classes.navbar}>
        <ScrollArea
          className={classes.navbarMain}
          type="hover"
          scrollbars="y"
          scrollHideDelay={0}
        >
          <LayoutSidebar userPermissions={userPermissions} />
        </ScrollArea>
        <div className={classes.footer}>
          <form action={logout}>
            <UnstyledButton w={"100%"} type="submit" className={classes.link}>
              <IconLogout className={classes.linkIcon} stroke={1.5} />
              <span>خروج</span>
            </UnstyledButton>
          </form>
        </div>
      </LayoutNavbar>
      <LayoutMain h={0}>{children}</LayoutMain>
    </LayoutShell>
  );
}
