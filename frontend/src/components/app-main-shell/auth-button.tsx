import Link from "next/link";
import {useMemo} from "react";
import {UnstyledButton, Skeleton} from "@mantine/core";
import {useInit} from "@/hooks/data/init";
import classes from "./auth-buttons.module.css";

type Links = {
  readonly label: string;
  readonly href: string;
};

const AUTH_LINKS: Links[] = [
  {
    label: "ورود",
    href: "/auth/login",
  },
  {
    label: "عضویت",
    href: "/auth/register",
  },
];

export function AuthButtons() {
  const {data, isLoading} = useInit();
  const status = data?.status;

  const LINKS = useMemo(() => {
    if (isLoading) {
      return (
        <Skeleton>
          <UnstyledButton>XXXXXXXXXX</UnstyledButton>
        </Skeleton>
      );
    }
    if (status === "unauthenticated") {
      return AUTH_LINKS.map((link) => {
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
    }
    if (status === "authenticated") {
      return (
        <UnstyledButton
          className={classes.control}
          component={Link}
          href={"/dashboard"}
        >
          پنل کاربری
        </UnstyledButton>
      );
    }
  }, [status, isLoading]);

  return LINKS;
}
