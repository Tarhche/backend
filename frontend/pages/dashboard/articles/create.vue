<script setup>
definePageMeta({layout:"dashboard"})

const  sendData = async (value) => {
  const cookie = useCookie("jwt")
  const url = useApiUrlResolver().resolve("api/dashboard/articles")

  const {data:data ,status, error}  = await useFetch(url, {
    method:"post",
    headers: {
      Authorization: `Bearer ${cookie.value}`,
    },
    body:value
  })

  if (status.value === "success") {
    useRouter().go(-1)
    await refreshNuxtData()
  }
}
</script>

<template>
  <div class="container">
    <div class="row mt-4">
      <article class="article col col-md-10 mx-auto">
        <dashboard-articles-create  @send="sendData"/>
      </article>
    </div>
  </div>
</template>

<style scoped>
.article {
  margin-top: 10vh;
}
</style>