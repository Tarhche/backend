import {
  Table,
  TableTr,
  TableTd,
  TableTh,
  TableThead,
  TableTbody,
  Skeleton,
} from "@mantine/core";

type Props = {
  headers: string[];
  rowsLength?: number;
};

export function TableSkeleton({headers, rowsLength}: Props) {
  const comments = new Array(rowsLength || 5).fill(1).map((_, i) => i);

  return (
    <Table verticalSpacing={"sm"} mt={"sm"}>
      <TableThead>
        <TableTr>
          {headers.map((h) => {
            return <TableTh key={h}>{h}</TableTh>;
          })}
        </TableTr>
      </TableThead>
      <TableTbody>
        {comments.map((comment) => {
          return (
            <TableTr key={comment}>
              {headers.map((h) => {
                return (
                  <TableTd key={h}>
                    <Skeleton w={"100%"} h={25} />
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
