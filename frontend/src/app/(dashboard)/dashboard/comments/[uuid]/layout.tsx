import {ReactNode} from "react";
import {Box} from "@mantine/core";
import {DashboardBreadcrumbs} from "@/features/breadcrumbs/components/breadcrumbs";
import {APP_PATHS} from "@/lib/app-paths";

function EditCommentLayout({children}: {children: ReactNode}) {
  return (
    <>
      <DashboardBreadcrumbs
        crumbs={[
          {
            label: "کامنت کاربران",
            href: APP_PATHS.dashboard.comments.index,
          },
          {
            label: "ویرایش کامنت",
          },
        ]}
      />
      <Box mt="md">{children}</Box>
    </>
  );
}

export default EditCommentLayout;
