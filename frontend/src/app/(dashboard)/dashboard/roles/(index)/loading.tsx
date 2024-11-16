import {RolesTableSkeleton} from "@/features/dashboard/roles-table";
import {BreadcrumbSkeleton} from "@/components/breadcrumb-skeleton";

function RolesPageLoading() {
  return (
    <>
      <BreadcrumbSkeleton />
      <RolesTableSkeleton />
    </>
  );
}

export default RolesPageLoading;
