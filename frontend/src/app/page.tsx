import {API} from "@/lib/api";
import Link from "next/link";
import {
  Stack,
  Grid,
  GridCol,
  List,
  ListItem,
  Group,
  Anchor,
} from "@mantine/core";
import {ArticleCardVertical} from "@/features/home-page/article-card-vertical";
import classes from "./page.module.css";

interface ArticleType {
  uuid: string;
  cover: string;
  title: string;
  author: {
    name: string;
    avatar: string;
  };
  published_at: string;
  excerpt: string;
  tags: string[];
}

export default async function HomePage() {
  const homePageData = (await API.get("home")).data;
  const latestArticles = homePageData.all as ArticleType[];
  const popularArticles = homePageData.popular as ArticleType[];

  return (
    <Grid gutter={50}>
      <GridCol
        span={{
          base: 12,
          md: 6,
        }}
      >
        <h2 className={classes.headingWithBorder}>
          <span>جدیدترین ها</span>
        </h2>
        <Stack gap={"sm"}>
          {latestArticles.map((la) => {
            return (
              <ArticleCardVertical
                key={la.uuid}
                article={{
                  thumbnail: la.cover,
                  title: la.title,
                  subtitle: la.excerpt,
                  publishedDate: la.published_at,
                  slug: la.uuid,
                }}
              />
            );
          })}
        </Stack>
      </GridCol>
      <GridCol
        span={{
          base: 12,
          md: 6,
        }}
      >
        <h2 className={classes.headingWithBorder}>
          <span>جدیدترین ها</span>
        </h2>
        <Stack gap={"sm"}>
          <List listStyleType="numbered">
            {popularArticles.map((article) => {
              return (
                <ListItem mb={"sm"} key={article.uuid}>
                  <Anchor
                    underline="never"
                    component={Link}
                    href={`articles/${article.uuid}`}
                  >
                    {article.title}
                  </Anchor>
                  <Group ms={"sm"} gap={"xs"}>
                    {article.tags.map((tag) => {
                      return (
                        <Anchor
                          key={tag}
                          component={Link}
                          href={`hashtags/${tag}`}
                        >
                          #{tag}
                        </Anchor>
                      );
                    })}
                  </Group>
                </ListItem>
              );
            })}
          </List>
        </Stack>
      </GridCol>
    </Grid>
  );
}
