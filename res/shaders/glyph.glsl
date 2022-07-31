//shader:vertex
#version 410

//aVertPos must be in the range [0,1]
layout(location=0) in vec3 aVertPos;

//Instanced
layout(location=1) in vec2 aUV0;
layout(location=2) in vec2 aUVSize;
layout(location=3) in vec4 aVertColor;
layout(location=4) in vec3 aModelPos;
layout(location=5) in vec2 aModelScale;

out vec2 v2fUV0;
out vec4 v2fColor;
out vec3 v2fFragPos;

uniform mat4 projViewMat;

void main()
{
    mat4 modelMat = mat4(
        aModelScale.x,  0.0,            0.0,            0.0, 
        0.0,            aModelScale.y,  0.0,            0.0, 
        0.0,            0.0,            1.0,            0.0, 
        aModelPos.x,    aModelPos.y,    aModelPos.z,    1.0
    );

    v2fUV0 = aUV0 + aVertPos.xy * aUVSize;
    v2fColor = aVertColor;

    gl_Position = projViewMat * modelMat * vec4(aVertPos, 1.0);
}

//shader:fragment
#version 410

in vec2 v2fUV0;
in vec4 v2fColor;

out vec4 fragColor;

uniform sampler2D diffTex;
uniform int drawBounds;

void main()
{
    vec4 texColor = texelFetch(diffTex, ivec2(v2fUV0), 0);
    // This part highlights the full region of the char
    if (texColor.r == 0 && drawBounds != 0)
    {
        fragColor = vec4(0,1,0,0.25);
    }
    else
    {
        fragColor = vec4(v2fColor.rgb, texColor.r*v2fColor.a);
    }
} 
