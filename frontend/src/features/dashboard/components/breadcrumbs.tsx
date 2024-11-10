import {Breadcrumbs} from "@/components/breadcrumbs";
import {APP_PATHS} from "@/lib/app-paths";

type Props = {
  crumbs: {
    label: string;
    href?: string;
  }[];
};

export function DashboardBreadcrumbs({crumbs}: Props) {
  return (
    <Breadcrumbs
      crumbs={[
        {
          label: "داشبرد",
          href: `${APP_PATHS.dashboard.index}`,
        },
        ...crumbs,
      ]}
    />
  );
}
