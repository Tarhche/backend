import Link from "next/link";
import {Title, Text, Button, Container, Group} from "@mantine/core";
import classes from "./not-found.module.css";

function NotFoundPage() {
  return (
    <Container className={classes.root}>
      <div className={classes.label}>404</div>
      <Title className={classes.title}>یه جای مخفی پیدا کردی!</Title>
      <Text c="dimmed" size="lg" ta="center" className={classes.description}>
        متاسفانه آدرس اشتباهی را دنبال کرده اید یا صفحه ای که به دنبال آن بودید
        به یک آدرس دیگری منتقل شده
      </Text>
      <Group justify="center">
        <Button variant="subtle" size="md" component={Link} href={"/"}>
          منو به خانه ببر
        </Button>
      </Group>
    </Container>
  );
}

export default NotFoundPage;
