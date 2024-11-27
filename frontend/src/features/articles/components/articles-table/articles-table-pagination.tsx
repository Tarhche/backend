"use client";
import {useSearchParams, usePathname} from "next/navigation";
import Link from "next/link";
import {Pagination} from "@mantine/core";

type Props = {
  current: number;
  total: number;
};

export function ArticlesPagination({total, current}: Props) {
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const params = new URLSearchParams(searchParams);
  params.delete("page");

  return (
    <Pagination
      total={total}
      value={current}
      size={"md"}
      getItemProps={(page) => ({
        component: Link,
        href: `${pathname}?page=${page}&${params.toString()}`,
      })}
      getControlProps={(control) => {
        switch (control) {
          case "first":
            return {
              component: Link,
              href: `${pathname}?page=1&${params.toString()}`,
              scroll: false,
            };
          case "last":
            return {
              component: Link,
              href: `${pathname}?page=${Math.ceil(total)}&${params.toString()}`,
              scroll: false,
            };
          case "next":
            return {
              component: Link,
              href: `${pathname}?page=${
                current === total ? total : current + 1
              }&${params.toString()}`,
              scroll: false,
            };
          case "previous":
            return {
              component: Link,
              href: `${pathname}?page=${
                current === 1 ? 1 : current - 1
              }&${params.toString()}`,
              scroll: false,
            };
        }
      }}
    />
  );
}
