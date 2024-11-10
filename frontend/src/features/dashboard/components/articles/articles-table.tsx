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
  Button,
  rem,
} from "@mantine/core";
import {ArticlesPagination} from "./articles-table-pagination";
import {
  IconEye,
  IconPencil,
  IconFilePlus,
  IconTrash,
} from "@tabler/icons-react";
import {fetchArticles} from "@/dal/articles";
import {dateFromNow} from "@/lib/date-and-time";
import {APP_PATHS} from "@/lib/app-paths";

type Props = {
  page: number | string;
};

export async function ArticlesTable({page}: Props) {
  const articlesResponse = await fetchArticles({
    params: {
      page: page,
    },
  });
  const articles = articlesResponse.items;
  const {total_pages, current_page} = articlesResponse.paginationResponse;

  const tableActions = [
    {
      tooltipLabel: "بازدید کردن مقاله",
      Icon: IconEye,
      color: "blue",
      href: (uuid: string) => APP_PATHS.articles.detail(uuid),
    },
    {
      tooltipLabel: "ویرایش کردن مقاله",
      Icon: IconPencil,
      color: "blue",
      href: (uuid: string) => APP_PATHS.dashboard.articles.edit(uuid),
    },
  ];

  return (
    <>
      <Group justify="flex-end">
        <Button
          variant="light"
          component={Link}
          leftSection={<IconFilePlus />}
          href={APP_PATHS.dashboard.articles.new}
        >
          مقاله جدید
        </Button>
      </Group>
      <Table verticalSpacing={"sm"} striped withRowBorders>
        <TableThead>
          <TableTr>
            <TableTh>#</TableTh>
            <TableTh>عنوان</TableTh>
            <TableTh>تاریخ انتشار</TableTh>
            <TableTh>عملیات</TableTh>
          </TableTr>
        </TableThead>
        <TableTbody>
          {articles.map((article: any, index: number) => {
            return (
              <TableTr key={article.uuid}>
                <TableTd>{index + 1}</TableTd>
                <TableTd>{article.title}</TableTd>
                <TableTd>{dateFromNow(article.published_at)}</TableTd>
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
                            href={href(article.uuid)}
                            aria-label={tooltipLabel}
                          >
                            <Icon style={{width: rem(20)}} stroke={1.5} />
                          </ActionIcon>
                        </Tooltip>
                      );
                    })}
                    <Tooltip label="حذف کردن مقاله" withArrow>
                      <ActionIcon
                        variant="light"
                        size="lg"
                        color="red"
                        aria-label="حذف کردن مقاله"
                      >
                        <IconTrash style={{width: rem(20)}} stroke={1.5} />
                      </ActionIcon>
                    </Tooltip>
                  </ActionIconGroup>
                </TableTd>
              </TableTr>
            );
          })}
        </TableTbody>
      </Table>
      <Group mt="md" mb={"lg"} justify="flex-end">
        <ArticlesPagination total={total_pages} current={current_page} />
      </Group>
    </>
  );
}
