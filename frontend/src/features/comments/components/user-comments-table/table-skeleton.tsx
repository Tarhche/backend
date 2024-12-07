import {TABLE_HEADERS} from "./table";
import {TableSkeleton} from "@/components/skeletons/table";

export function UserCommentsTableSkeleton() {
  return (
    <TableSkeleton
      columnsCount={TABLE_HEADERS.length}
      tableProps={{
        verticalSpacing: "sm",
      }}
    />
  );
}
