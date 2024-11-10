import {
  Table,
  TableTr,
  TableTd,
  TableTh,
  TableThead,
  TableTbody,
  Skeleton,
} from "@mantine/core";
import {TABLE_HEADERS} from "./comments-table";

export function CommentsTableSkeleton() {
  const commentsResponse = new Array(5).fill(1).map((_, i) => i);

  return (
    <Table verticalSpacing={"sm"} mt={"sm"}>
      <TableThead>
        <TableTr>
          {TABLE_HEADERS.map((h) => {
            return <TableTh key={h}>{h}</TableTh>;
          })}
        </TableTr>
      </TableThead>
      <TableTbody>
        {commentsResponse.map((comment) => {
          return (
            <TableTr key={comment}>
              {TABLE_HEADERS.map((h) => {
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
