import {
  Stack,
  Grid,
  GridCol,
  List,
  ListItem,
  Anchor,
  Skeleton,
  Group,
} from "@mantine/core";
import {VerticalArticleCardSkeleton} from "../article-card-vertical/skeleton";
import classes from "./featured-articles.module.css";

const POPULAR_POSTS_COUNT = [1, 2, 3] as const;

export function FeaturedArticlesSkeleton() {
  return (
    <>
      <Grid>
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
            <VerticalArticleCardSkeleton />
            <VerticalArticleCardSkeleton />
            <VerticalArticleCardSkeleton />
          </Stack>
        </GridCol>
        <GridCol
          span={{
            base: 12,
            lg: 6,
          }}
        >
          <h2 className={classes.headingWithBorder}>
            <span>پربازدید ترین ها</span>
          </h2>
          <Stack gap={"sm"}>
            <List listStyleType="none">
              {POPULAR_POSTS_COUNT.map((_, index) => {
                return (
                  <ListItem
                    mb={"xl"}
                    key={index}
                    styles={{
                      itemLabel: {
                        width: "100%",
                      },
                      itemWrapper: {
                        width: "70%",
                      },
                    }}
                  >
                    <Skeleton w={"100%"}>X</Skeleton>
                    <Group ms={"sm"}>
                      {POPULAR_POSTS_COUNT.map((tag) => {
                        return (
                          <Skeleton mt={10} w={"50px"} key={tag}>
                            <Anchor>###</Anchor>
                          </Skeleton>
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
    </>
  );
}
