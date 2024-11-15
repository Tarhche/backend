import {TableSkeleton} from "@/components/table-skeleton";

import {TABLE_HEADERS} from "./roles-table";

export function RolesTableSkeleton() {
  return <TableSkeleton headers={TABLE_HEADERS} />;
}
