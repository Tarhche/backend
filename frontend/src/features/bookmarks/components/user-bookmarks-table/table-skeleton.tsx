import {TableSkeleton} from "@/components/skeletons/table";
import {TABLE_HEADERS} from "./table";

export function UserBookmarksTableSkeleton() {
  return (
    <TableSkeleton
      columnsCount={TABLE_HEADERS.length}
      tableProps={{
        verticalSpacing: "sm",
      }}
    />
  );
}
