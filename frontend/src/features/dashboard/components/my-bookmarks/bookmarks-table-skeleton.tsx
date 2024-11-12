import {TABLE_HEADERS} from "./bookmarks-table";
import {TableSkeleton} from "../table-skeleton";

export function MyBookmarksTableSkeleton() {
  return <TableSkeleton headers={TABLE_HEADERS} />;
}
