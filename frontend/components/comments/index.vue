<template>
  <section>
        <h5 class="mb-3">
            <span class="fa-regular fa-comment"></span>
            <span class="mx-1">دیدگاه ها</span>
        </h5>

        <form v-if="params.isLogin" class="my-3">
            <textarea class="form-control mb-3" placeholder="دیدگاه خود را اینجا بنویسید" rows="3" required></textarea>
            <button class="btn btn-success" type="submit">ثبت دیدگاه</button>
        </form>

        <div v-else class="alert alert-light">
            <i class="fa-regular fa-bell fa-shake fa-xl"></i>
            <span class="mx-1">برای ثبت دیدگاه خود</span>
            <NuxtLink class="mx-1" href="/auth/register">ثبت نام کنید</NuxtLink>
            <span>یا</span>
            <NuxtLink class="mx-1" to="/auth/login">وارد شوید</NuxtLink>
        </div>
    <comments-item v-for="(item , index) in comments" :key="index" :data="item"  v-if="comments && comments.length && params.isLogin"/>
  </section>
</template>

<script lang="ts" setup>
const comments = [
  {
    img: 'https://mdbcdn.b-cdn.net/img/Photos/Avatars/img%20(3).webp',
    name: "افشار زند",
    time: "'2024/05/20'",
    text: "طراحان معمولا از لورم ایپسوم استفاده میکنند تا فقط به مشتری یا کار فرما نشان دهند که قالب طراحی شده بعد از اینکه متن در آن قرار میگرد چگونه خواهد بود و فونت ها و اندازه ها چگونه در نظر گرفته شده است.",
    sub:[
      {
        img: 'https://mdbcdn.b-cdn.net/img/Photos/Avatars/img%20(3).webp',
        name: "افشار زند",
        time: "'2024/05/20'",
        text: "طراحان معمولا از لورم ایپسوم استفاده میکنند تا فقط به مشتری یا کار فرما نشان دهند که قالب طراحی شده بعد از اینکه متن در آن قرار میگرد چگونه خواهد بود و فونت ها و اندازه ها چگونه در نظر گرفته شده است.",
        sub:[
          {
            img: 'https://mdbcdn.b-cdn.net/img/Photos/Avatars/img%20(3).webp',
            name: "عیسی خانزادی",
            time: "'2024/05/20'",
            text: "طراحان معمولا از لورم ایپسوم استفاده میکنند تا فقط به مشتری یا کار فرما نشان دهند که قالب طراحی شده بعد از اینکه متن در آن قرار میگرد چگونه خواهد بود و فونت ها و اندازه ها چگونه در نظر گرفته شده است."
          },
        ]
      },
      {
        img: 'https://mdbcdn.b-cdn.net/img/Photos/Avatars/img%20(3).webp',
        name: "افشار زند",
        time: "'2024/05/20'",
        text: "طراحان معمولا از لورم ایپسوم استفاده میکنند تا فقط به مشتری یا کار فرما نشان دهند که قالب طراحی شده بعد از اینکه متن در آن قرار میگرد چگونه خواهد بود و فونت ها و اندازه ها چگونه در نظر گرفته شده است."
      },
    ]
  },
  {
    img: 'https://mdbcdn.b-cdn.net/img/Photos/Avatars/img%20(3).webp',
    name: "عیسی خانزادی",
    time: "'2024/05/20'",
    text: "طراحان معمولا از لورم ایپسوم استفاده میکنند تا فقط به مشتری یا کار فرما نشان دهند که قالب طراحی شده بعد از اینکه متن در آن قرار میگرد چگونه خواهد بود و فونت ها و اندازه ها چگونه در نظر گرفته شده است."
  },
  {
    img: 'https://mdbcdn.b-cdn.net/img/Photos/Avatars/img%20(3).webp',
    name: "مهدی خانزادی",
    time: "'2024/05/20'",
    text: "طراحان معمولا از لورم ایپسوم استفاده میکنند تا فقط به مشتری یا کار فرما نشان دهند که قالب طراحی شده بعد از اینکه متن در آن قرار میگرد چگونه خواهد بود و فونت ها و اندازه ها چگونه در نظر گرفته شده است."
  },
  {
    img: 'https://mdbcdn.b-cdn.net/img/Photos/Avatars/img%20(3).webp',
    name: "صادق خانزادی",
    time: "'2024/05/20'",
    text: "طراحان معمولا از لورم ایپسوم استفاده میکنند تا فقط به مشتری یا کار فرما نشان دهند که قالب طراحی شده بعد از اینکه متن در آن قرار میگرد چگونه خواهد بود و فونت ها و اندازه ها چگونه در نظر گرفته شده است."
  },
]
const params = reactive({
        isLogin: useAuth().isLogin(),
})
</script>
