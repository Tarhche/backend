import {
  Table,
  TableTr,
  TableTd,
  TableTh,
  TableThead,
  TableTbody,
  Skeleton,
  type TableProps,
} from "@mantine/core";
import {generateRange} from "@/lib/arrays";

type Props = {
  columnsCount?: number;
  rowsCount?: number;
  tableProps?: Omit<TableProps, "data">;
};

export function TableSkeleton(props: Props) {
  const {tableProps, columnsCount = 5, rowsCount = 5} = props;
  const columns = generateRange(columnsCount);
  const rows = generateRange(rowsCount);

  return (
    <Table {...tableProps}>
      <TableThead>
        <TableTr>
          {columns.map((column) => {
            return (
              <TableTh key={column}>
                <Skeleton w="auto" h={20} />
              </TableTh>
            );
          })}
        </TableTr>
      </TableThead>
      <TableTbody>
        {rows.map((row) => {
          return (
            <TableTr key={row}>
              {columns.map((column) => {
                return (
                  <TableTd key={column}>
                    <Skeleton w="auto" h={25} />
                  </TableTd>
                );
              })}
            </TableTr>
          );
        })}
      </TableTbody>
    </Table>
  );
}
