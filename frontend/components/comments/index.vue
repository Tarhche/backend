<template>
    <div>
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

        <section class="card mb-3">
            <div class="card-body d-flex flex-start">
                <img class="rounded-circle shadow-1-strong ms-3" src="https://mdbcdn.b-cdn.net/img/Photos/Avatars/img%20(3).webp" alt="avatar" width="65" height="65">
                <div class="flex-grow-1 flex-shrink-1">
                    <div class="d-flex justify-content-between align-items-center">
                    <p class="mb-1">
                        <span>افشار زند</span>
                        <span class="text-muted small">
                        <span class="fa-regular fa-clock mx-1"></span>
                        <time datetime="">{{ useTime().toAgo('2024/05/20') }}</time>
                        </span>
                    </p>
                    <div v-if="params.isLogin" class="text-nowrap">
                        <button class="btn text-danger btn-sm">
                            <i class="fas fa-trash fa-xs"></i>
                        </button>
                        <span>|</span>
                        <button class="btn btn-sm">
                        <i class="fas fa-reply fa-xs"></i>
                        </button>
                    </div>
                    </div>
                    <p class="small mb-0 pt-2 border-top">
                        طراحان معمولا از لورم ایپسوم
                         استفاده میکنند تا فقط به مشتری یا کار فرما نشان دهند که قالب طراحی شده بعد از اینکه متن
                         در آن قرار میگرد چگونه خواهد بود و فونت ها و اندازه ها چگونه در نظر گرفته شده است.
                    </p>
                </div>
            </div>
        </section>
    </div>
</template>

<script lang="ts" setup>
    const params = reactive({
        isLogin: useAuth().isLogin(),
    })
</script>