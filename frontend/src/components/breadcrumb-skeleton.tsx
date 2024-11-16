import {Breadcrumbs, Skeleton} from "@mantine/core";
import {generateRange} from "@/lib/arrays";
import classes from "./breadcrumbs.module.css";

type Props = {
  separator?: React.ReactNode;
  crumbsCount?: number;
};

export function BreadcrumbSkeleton({separator = "\\", crumbsCount = 3}: Props) {
  return (
    <Breadcrumbs
      separator={separator}
      classNames={{
        separator: classes.separator,
      }}
    >
      {generateRange(crumbsCount).map((i) => {
        return <Skeleton key={i} w={50} h={20} />;
      })}
    </Breadcrumbs>
  );
}
