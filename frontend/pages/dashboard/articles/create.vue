<template>
  <div class="container">
    <div class="row mt-4">
      <article class="article col col-md-10 mx-auto">
        <dashboard-articles-create/>
      </article>
    </div>
  </div>
</template>
<script setup>
definePageMeta({
  layout:"dashboard"
})
const url = useApiUrlResolver().resolve(`api/dashboard/articles`)
const token = "eyJhbGciOiJFUzUxMiIsInR5cCI6IkpXVCJ9.eyJhdWQiOlsiYWNjZXNzIl0sImV4cCI6MTcwOTE1NDcwNiwiaWF0IjoxNzA5MTUzODA2LCJuYmYiOjE3MDkxNTM4MDYsInN1YiI6IjAxOGRlMTBkLWI3YTctNzg4ZS05NGMzLWIzYjlmMDZiZTVjYSJ9.ATU0KPXjhxllPovE-kxGo_wFzcskP31-Aa3NJx1T7TMax32g7EWNH_B5frdopg8bjvpOqle4IxbILssK_AGG82YhAWFwtPJLWqQkLqToeYK1Da22ar8JURQjhSsO4C4RNJsqiw0ADPvPyU-vqz0jiHCGQAUzvU-dEK4DgW7mhDnqCAQW"
  const {data:response , error} = await useFetch(url,{
    method:"post" ,
    headers:{
      // Authorization: `Bearer ${token.value}`,
      Authorization: `Bearer ${token}`,

    },
    body:{
      cover : "سلام",
      title:"سلام",
      body:"سلام",
      excerpt:"سلام"
    }
  })
  console.log(response , error)

</script>
<style scoped>
.article{
  margin-top: 10vh;
}
</style>