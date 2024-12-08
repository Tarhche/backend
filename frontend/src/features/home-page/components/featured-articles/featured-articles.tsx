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
import {VerticalArticleCard} from "../article-card-vertical";
import {fetchHomePageData} from "@/dal/public/home";
import classes from "./featured-articles.module.css";

export async function FeaturedArticles() {
  const homePageData = await fetchHomePageData();
  const latestArticles = homePageData.all;
  const popularArticles = homePageData.popular;

  return (
    <Grid gutter={50}>
      <GridCol
        span={{
          base: 12,
          md: 8,
        }}
      >
        <h2 className={classes.headingWithBorder}>
          <span>جدیدترین ها</span>
        </h2>
        <Stack gap={"sm"}>
          {latestArticles.map((la) => {
            return (
              <VerticalArticleCard
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
          md: 4,
        }}
      >
        <h2 className={classes.headingWithBorder}>
          <span>پربازدیدترین ها</span>
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
