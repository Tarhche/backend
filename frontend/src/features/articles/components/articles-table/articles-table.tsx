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
  Badge,
  rem,
  type MantineColor,
} from "@mantine/core";
import {PermissionGuard} from "@/components/permission-guard";
import {ArticlesPagination} from "./articles-table-pagination";
import {ArticleDeleteButton} from "./article-delete-button";
import {
  IconEye,
  IconPencil,
  IconFilePlus,
  Icon,
  IconProps,
} from "@tabler/icons-react";
import {fetchAllArticles} from "@/dal/private/articles";
import {dateFromNow} from "@/lib/date-and-time";
import {APP_PATHS} from "@/lib/app-paths";
import {type Permissions} from "@/lib/app-permissions";

type Props = {
  page: number | string;
};

type TableAction = {
  tooltipLabel: string;
  Icon: React.ForwardRefExoticComponent<IconProps & React.RefAttributes<Icon>>;
  color: MantineColor;
  allowedPermissions: Permissions[];
  href: (uuid: string) => string;
  disabled: (...args: any[]) => boolean;
};

export async function ArticlesTable({page}: Props) {
  const articlesResponse = await fetchAllArticles({
    params: {
      page: page,
    },
  });
  const articles = articlesResponse.items;
  const {total_pages, current_page} = articlesResponse.pagination;

  const tableActions: TableAction[] = [
    {
      tooltipLabel: "بازدید کردن مقاله",
      Icon: IconEye,
      color: "blue",
      allowedPermissions: [],
      href: (uuid: string) => APP_PATHS.articles.detail(uuid),
      disabled: (published: boolean) => published,
    },
    {
      tooltipLabel: "ویرایش کردن مقاله",
      Icon: IconPencil,
      color: "blue",
      allowedPermissions: ["articles.update"],
      href: (uuid: string) => APP_PATHS.dashboard.articles.edit(uuid),
      disabled: () => false,
    },
  ];

  return (
    <>
      <PermissionGuard allowedPermissions={["articles.create"]}>
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
      </PermissionGuard>
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
          {articles.length === 0 && (
            <TableTr>
              <TableTd colSpan={4} ta={"center"}>
                مقاله های وجود ندارد
              </TableTd>
            </TableTr>
          )}
          {articles.map((article: any, index: number) => {
            const isPublished = new Date(article.published_at).getDate() !== 1;

            return (
              <TableTr key={article.uuid}>
                <TableTd>{index + 1}</TableTd>
                <TableTd>{article.title}</TableTd>
                <TableTd>
                  {isPublished ? (
                    dateFromNow(article.published_at)
                  ) : (
                    <Badge color="yellow" variant="light">
                      منتشر نشده
                    </Badge>
                  )}
                </TableTd>
                <TableTd>
                  <ActionIconGroup>
                    {tableActions.map(
                      ({
                        Icon,
                        tooltipLabel,
                        color,
                        href,
                        allowedPermissions,
                        disabled,
                      }) => {
                        return (
                          <PermissionGuard
                            key={tooltipLabel}
                            allowedPermissions={allowedPermissions}
                          >
                            <Tooltip label={tooltipLabel} withArrow>
                              <ActionIcon
                                component={Link}
                                variant="light"
                                size="lg"
                                color={color}
                                href={href(article.uuid)}
                                disabled={disabled(isPublished === false)}
                                aria-label={tooltipLabel}
                              >
                                <Icon style={{width: rem(20)}} stroke={1.5} />
                              </ActionIcon>
                            </Tooltip>
                          </PermissionGuard>
                        );
                      },
                    )}
                    <PermissionGuard allowedPermissions={["articles.delete"]}>
                      <ArticleDeleteButton
                        articleID={article.uuid}
                        articleTitle={article.title}
                      />
                    </PermissionGuard>
                  </ActionIconGroup>
                </TableTd>
              </TableTr>
            );
          })}
        </TableTbody>
      </Table>
      {articles.length >= 1 && (
        <Group mt="md" mb="xl" justify="flex-end">
          <ArticlesPagination total={total_pages} current={current_page} />
        </Group>
      )}
    </>
  );
}
