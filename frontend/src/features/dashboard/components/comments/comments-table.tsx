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
import {CommentsPagination} from "./comments-table-pagination";
import {CommentDeleteButton} from "./comment-delete-button";
import {IconEye, IconPencil} from "@tabler/icons-react";
import {fetchAllComments} from "@/dal/comments";
import {dateFromNow, isGregorianStartDateTime} from "@/lib/date-and-time";
import {APP_PATHS} from "@/lib/app-paths";

export const TABLE_HEADERS = [
  "#",
  "کامنت",
  "تاریخ انتشار",
  "تاریخ ثبت",
  "نویسنده",
  "عملیات",
];

type Props = {
  page: number | string;
};

export async function CommentsTable({page}: Props) {
  const commentsResponse = await fetchAllComments({
    params: {
      page: page,
    },
  });
  const comments = commentsResponse.items;
  const {total_pages, current_page} = commentsResponse.pagination;

  const tableActions = [
    {
      tooltipLabel: "بازدید کردن کامنت",
      Icon: IconEye,
      color: "blue",
      href: (comment: any) => APP_PATHS.articles.detail(comment.object_uuid),
    },
    {
      tooltipLabel: "ویرایش کردن کامنت",
      Icon: IconPencil,
      color: "blue",
      href: (comment: any) => APP_PATHS.dashboard.comments.edit(comment.uuid),
    },
  ];

  return (
    <>
      <Table verticalSpacing={"sm"} striped withRowBorders>
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
              <TableTd colSpan={TABLE_HEADERS.length} ta={"center"}>
                کامنتی وجود ندارد
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
                    dateFromNow(comment.approved_at)
                  ) : (
                    <Badge color="yellow" variant="light">
                      تایید نشده
                    </Badge>
                  )}
                </TableTd>
                <TableTd>{dateFromNow(comment.created_at)}</TableTd>
                <TableTd>{comment.author.name}</TableTd>
                <TableTd>
                  <ActionIconGroup>
                    {tableActions.map(({Icon, tooltipLabel, color, href}) => {
                      return (
                        <Tooltip
                          key={tooltipLabel}
                          label={tooltipLabel}
                          withArrow
                        >
                          <ActionIcon
                            component={Link}
                            variant="light"
                            size="lg"
                            color={color}
                            href={href(comment)}
                            aria-label={tooltipLabel}
                          >
                            <Icon style={{width: rem(20)}} stroke={1.5} />
                          </ActionIcon>
                        </Tooltip>
                      );
                    })}
                    <CommentDeleteButton
                      commentID={comment.uuid}
                      commentMessage={comment.body}
                    />
                  </ActionIconGroup>
                </TableTd>
              </TableTr>
            );
          })}
        </TableTbody>
      </Table>
      {comments.length >= 1 && (
        <Group mt="md" mb={"lg"} justify="flex-end">
          <CommentsPagination total={total_pages} current={current_page} />
        </Group>
      )}
    </>
  );
}
