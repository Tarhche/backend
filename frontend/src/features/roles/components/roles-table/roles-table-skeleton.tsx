import {TableSkeleton} from "@/components/skeletons/table";
import {TABLE_HEADERS} from "./roles-table";

export function RolesTableSkeleton() {
  return (
    <TableSkeleton
      columnsCount={TABLE_HEADERS.length}
      tableProps={{
        verticalSpacing: "sm",
      }}
    />
  );
}
