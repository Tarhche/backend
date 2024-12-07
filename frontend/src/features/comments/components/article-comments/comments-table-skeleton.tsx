import {TABLE_HEADERS} from "./comments-table";
import {TableSkeleton} from "@/components/skeletons/table";

export function CommentsTableSkeleton() {
  return (
    <TableSkeleton
      columnsCount={TABLE_HEADERS.length}
      tableProps={{
        verticalSpacing: "sm",
      }}
    />
  );
}
