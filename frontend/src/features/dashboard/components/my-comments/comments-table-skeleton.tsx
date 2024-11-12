import {TABLE_HEADERS} from "./comments-table";
import {TableSkeleton} from "../table-skeleton";

export function MyCommentsTableSkeleton() {
  return <TableSkeleton headers={TABLE_HEADERS} />;
}
