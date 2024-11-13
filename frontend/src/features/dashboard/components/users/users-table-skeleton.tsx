import {TABLE_HEADERS} from "./users-table";
import {TableSkeleton} from "../table-skeleton";

export function UsersTableSkeleton() {
  return <TableSkeleton headers={TABLE_HEADERS} />;
}
