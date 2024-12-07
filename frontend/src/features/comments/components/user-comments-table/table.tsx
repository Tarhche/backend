import Link from "next/link";
import {
  Table,
  TableTr,
  TableTd,
  TableTh,
  TableThead,
  TableTbody,
  ActionIcon,
  ActionIconGroup,
  Tooltip,
  Group,
  Badge,
  rem,
} from "@mantine/core";
import {PermissionGuard} from "@/components/permission-guard";
import {Pagination} from "@/components/pagination";
import {DeleteButton} from "./delete-button";
import {IconEye} from "@tabler/icons-react";
import {fetchUserComments} from "@/dal/private/comments";
import {dateFromNow, isGregorianStartDateTime} from "@/lib/date-and-time";
import {APP_PATHS} from "@/lib/app-paths";

export const TABLE_HEADERS = ["#", "کامنت", "وضعیت", "تاریخ ثبت", "عملیات"];

type Props = {
  page: number | string;
};

export async function UserCommentsTable({page}: Props) {
  const commentsResponse = await fetchUserComments({
    params: {
      page: page,
    },
  });
  const comments = commentsResponse.items;
  const {total_pages, current_page} = commentsResponse.pagination;

  return (
    <>
      <Table verticalSpacing="sm" striped withRowBorders>
        <TableThead>
          <TableTr>
            {TABLE_HEADERS.map((h) => {
              return <TableTh key={h}>{h}</TableTh>;
            })}
          </TableTr>
        </TableThead>
        <TableTbody>
          {comments.length === 0 && (
            <TableTr>
              <TableTd colSpan={TABLE_HEADERS.length} ta="center">
                هنوز کامنتی را ثبت نکرده اید
              </TableTd>
            </TableTr>
          )}
          {comments.map((comment: any, index: number) => {
            const isApproved = !isGregorianStartDateTime(comment.approved_at);

            return (
              <TableTr key={comment.uuid}>
                <TableTd>{index + 1}</TableTd>
                <TableTd>{comment.body}</TableTd>
                <TableTd>
                  {isApproved ? (
                    <Badge color="green" variant="light">
                      تایید شده
                    </Badge>
                  ) : (
                    <Badge color="yellow" variant="light">
                      در انتظار تایید
                    </Badge>
                  )}
                </TableTd>
                <TableTd>{dateFromNow(comment.created_at)}</TableTd>
                <TableTd>
                  <ActionIconGroup>
                    <Tooltip label="بازدید کردن کامنت" withArrow>
                      <ActionIcon
                        variant="light"
                        size="lg"
                        color="blue"
                        aria-label="بازدید کردن کامنت"
                        component={Link}
                        href={`${APP_PATHS.articles.detail(comment.object_uuid)}`}
                      >
                        <IconEye style={{width: rem(20)}} stroke={1.5} />
                      </ActionIcon>
                    </Tooltip>
                    <PermissionGuard
                      allowedPermissions={["self.comments.delete"]}
                    >
                      <DeleteButton
                        commentID={comment.uuid}
                        commentMessage={comment.body}
                      />
                    </PermissionGuard>
                  </ActionIconGroup>
                </TableTd>
              </TableTr>
            );
          })}
        </TableTbody>
      </Table>
      {comments.length >= 1 && (
        <Group mt="md" mb="lg" justify="flex-end">
          <Pagination total={total_pages} current={current_page} />
        </Group>
      )}
    </>
  );
}
